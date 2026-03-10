(function() {
    var jwtToken = localStorage.getItem('linkterm_token');
    if (!jwtToken) {
        window.location.href = '/';
        return;
    }

    var term = null;
    var fitAddon = null;
    var ws = null;
    var sessionId = localStorage.getItem('linkterm_session_id') || null;
    var agentId = null;
    var ctrlActive = false;
    var lastHiddenTime = 0;
    var pingInterval = null;
    var reconnecting = false;
    var reconnectTimer = null;
    var reconnectAttempts = 0;
    var maxReconnectAttempts = 3;
    var bufferReplaying = false;

    var statusBar = document.getElementById('statusBar');
    var statusIcon = document.getElementById('statusIcon');
    var statusText = document.getElementById('statusText');
    var reconnectBar = document.getElementById('reconnectBar');
    var reconnectReason = document.getElementById('reconnectReason');
    var reconnectActions = document.getElementById('reconnectActions');
    var menuOverlay = document.getElementById('menuOverlay');

    var themes = {
        dark: {
            background: '#1a1b26', foreground: '#c0caf5', cursor: '#c0caf5',
            selectionBackground: '#33467c',
            black: '#15161e', red: '#f7768e', green: '#9ece6a', yellow: '#e0af68',
            blue: '#7aa2f7', magenta: '#bb9af7', cyan: '#7dcfff', white: '#a9b1d6'
        },
        light: {
            background: '#fafafa', foreground: '#383a42', cursor: '#526eff',
            selectionBackground: '#d7d7ff',
            black: '#383a42', red: '#e45649', green: '#50a14f', yellow: '#c18401',
            blue: '#4078f2', magenta: '#a626a4', cyan: '#0184bc', white: '#fafafa'
        },
        dracula: {
            background: '#282a36', foreground: '#f8f8f2', cursor: '#f8f8f2',
            selectionBackground: '#44475a',
            black: '#21222c', red: '#ff5555', green: '#50fa7b', yellow: '#f1fa8c',
            blue: '#bd93f9', magenta: '#ff79c6', cyan: '#8be9fd', white: '#f8f8f2'
        }
    };
    var currentTheme = localStorage.getItem('linkterm_theme') || 'dark';

    var fontPresets = {
        small:  { size: 13, line: 1.25 },
        medium: { size: 16, line: 1.2 },
        large:  { size: 19, line: 1.2 }
    };
    var currentFontPreset = localStorage.getItem('linkterm_fontpreset') || 'medium';
    if (!fontPresets[currentFontPreset]) currentFontPreset = 'medium';

    var lastDisconnectReason = '';
    var connectSeq = 0;

    initTerminal();
    connectSeq++;
    connectToSession();

    function initTerminal() {
        term = new window.Terminal({
            cursorBlink: true,
            cursorStyle: 'bar',
            fontSize: fontPresets[currentFontPreset].size,
            lineHeight: fontPresets[currentFontPreset].line,
            letterSpacing: 0,
            fontFamily: '"SF Mono", "Fira Code", "Cascadia Code", Menlo, Monaco, "Courier New", monospace',
            fontWeight: '400',
            fontWeightBold: '600',
            theme: themes[currentTheme] || themes.dark,
            allowProposedApi: true,
            drawBoldTextInBrightColors: true,
            minimumContrastRatio: 4.5
        });

        fitAddon = new window.FitAddon.FitAddon();
        term.loadAddon(fitAddon);

        var webLinksAddon = new window.WebLinksAddon.WebLinksAddon();
        term.loadAddon(webLinksAddon);

        term.open(document.getElementById('terminalContainer'));
        fitAddon.fit();

        document.body.style.backgroundColor = (themes[currentTheme] || themes.dark).background;

        term.onData(function(data) {
            if (ctrlActive) {
                var code = data.charCodeAt(0);
                if (code >= 97 && code <= 122) {
                    data = String.fromCharCode(code - 96);
                }
                ctrlActive = false;
                updateCtrlBtn();
            }
            sendInput(data);
        });

        window.addEventListener('resize', function() {
            fitAddon.fit();
            sendResize();
        });

        new ResizeObserver(function() {
            fitAddon.fit();
            sendResize();
        }).observe(document.getElementById('terminalContainer'));

        setupToolbar();
        setupMenu();
        setupVisibility();
        setupReconnectActions();
    }

    /** connectToSession 先验证 JWT，再恢复或创建会话 */
    function connectToSession() {
        var seq = connectSeq;
        setStatus('connecting', '正在验证...');
        fetch('/api/agents', {
            headers: { 'Authorization': 'Bearer ' + jwtToken }
        })
        .then(function(resp) {
            if (seq !== connectSeq) return;
            if (resp.status === 401) {
                redirectToLogin();
                return;
            }
            return resp.json();
        })
        .then(function(agents) {
            if (seq !== connectSeq) return;
            if (!agents || agents.length === 0) {
                setStatus('disconnected', 'Mac 离线');
                showReconnectBar('请确认 Agent 已启动并连接到服务端', true);
                return;
            }
            agentId = agents[0].id;

            if (sessionId) {
                setStatus('connecting', '恢复已有会话...');
                connectWebSocket(function onFail() {
                    if (seq !== connectSeq) return;
                    sessionId = null;
                    localStorage.removeItem('linkterm_session_id');
                    findOrCreateSession(agentId);
                });
            } else {
                findOrCreateSession(agentId);
            }
        })
        .catch(function(err) {
            if (seq !== connectSeq) return;
            setStatus('disconnected', '连接失败');
            showReconnectBar('无法连接到服务端: ' + err.message, true);
        });
    }

    /** findOrCreateSession 查找可复用的已有会话，没有则创建 */
    function findOrCreateSession(agId) {
        var seq = connectSeq;
        setStatus('connecting', '正在查找会话...');
        return fetch('/api/sessions?agent_id=' + agId, {
            headers: { 'Authorization': 'Bearer ' + jwtToken }
        })
        .then(function(resp) {
            if (seq !== connectSeq) return;
            if (resp.status === 401) { redirectToLogin(); return; }
            return resp.json();
        })
        .then(function(sessions) {
            if (seq !== connectSeq) return;
            if (!sessions) return;
            var reusable = null;
            for (var i = 0; i < sessions.length; i++) {
                var st = sessions[i].status;
                if (st === 'active' || st === 'detached') {
                    reusable = sessions[i];
                    break;
                }
            }
            if (reusable) {
                sessionId = reusable.id;
                localStorage.setItem('linkterm_session_id', sessionId);
                setStatus('connecting', '恢复已有会话...');
                connectWebSocket(function onFail() {
                    if (seq !== connectSeq) return;
                    sessionId = null;
                    localStorage.removeItem('linkterm_session_id');
                    createSession(agId);
                });
            } else {
                createSession(agId);
            }
        });
    }

    var skipInitialResize = false;

    function createSession(agId) {
        var seq = connectSeq;
        setStatus('connecting', '正在创建...');
        fetch('/api/sessions', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + jwtToken
            },
            body: JSON.stringify({
                agent_id: agId,
                cols: term.cols,
                rows: term.rows
            })
        })
        .then(function(resp) {
            if (seq !== connectSeq) return;
            if (resp.status === 401) { redirectToLogin(); return; }
            return resp.json();
        })
        .then(function(data) {
            if (seq !== connectSeq) return;
            if (!data) return;
            sessionId = data.session_id;
            localStorage.setItem('linkterm_session_id', sessionId);
            skipInitialResize = true;
            connectWebSocket();
        })
        .catch(function(err) {
            if (seq !== connectSeq) return;
            setStatus('disconnected', '创建终端失败: ' + err.message);
        });
    }

    function redirectToLogin() {
        localStorage.removeItem('linkterm_token');
        localStorage.removeItem('linkterm_session_id');
        window.location.href = '/';
    }

    function showReconnectBar(reason, showActions) {
        reconnectReason.textContent = reason || '';
        if (showActions) {
            reconnectActions.classList.remove('hidden');
        } else {
            reconnectActions.classList.add('hidden');
        }
        reconnectBar.classList.remove('hidden');
    }

    function hideReconnectBar() {
        reconnectBar.classList.add('hidden');
        reconnectActions.classList.add('hidden');
    }

    function closeWebSocket() {
        if (ws) {
            var old = ws;
            ws = null;
            old.onopen = null;
            old.onmessage = null;
            old.onclose = null;
            old.onerror = null;
            try {
                if (old.readyState === WebSocket.OPEN || old.readyState === WebSocket.CONNECTING) {
                    old.close();
                }
            } catch (e) {}
        }
        stopPing();
    }

    function connectWebSocket(onFail) {
        closeWebSocket();

        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        var url = protocol + '//' + window.location.host + '/ws/terminal/' + sessionId + '?token=' + jwtToken;

        ws = new WebSocket(url);
        ws.binaryType = 'arraybuffer';
        var handshakeOk = false;

        var stableResetDone = false;

        ws.onopen = function() {
            handshakeOk = true;
            setStatus('connected', '已连接');
            hideReconnectBar();
            reconnecting = false;
            stableResetDone = false;
            if (skipInitialResize) {
                skipInitialResize = false;
            } else {
                sendResize();
            }
            startPing();
            term.focus();
        };

        ws.onmessage = function(e) {
            if (!stableResetDone) {
                stableResetDone = true;
                reconnectAttempts = 0;
            }
            if (e.data instanceof ArrayBuffer) {
                term.write(new Uint8Array(e.data));
                if (bufferReplaying) {
                    bufferReplaying = false;
                    setTimeout(function() {
                        term.write('\x1b[2J\x1b[H');
                        sendResize();
                        setStatus('connected', '已连接');
                    }, 50);
                }
            } else {
                var msg = JSON.parse(e.data);
                handleControlMessage(msg);
            }
        };

        ws.onclose = function(e) {
            stopPing();
            if (!handshakeOk && onFail) {
                onFail();
                return;
            }
            if (!reconnecting && sessionId) {
                lastDisconnectReason = formatCloseReason(e.code, e.reason);
                setStatus('disconnected', '连接已断开');
                scheduleReconnect();
            }
        };

        ws.onerror = function() {
            stopPing();
        };
    }

    function handleControlMessage(msg) {
        switch (msg.type) {
            case 'buffered':
                setStatus('connecting', '恢复中...');
                bufferReplaying = true;
                break;
            case 'closed':
                setStatus('disconnected', '终端已结束 (exit ' + msg.exit_code + ')');
                sessionId = null;
                localStorage.removeItem('linkterm_session_id');
                showReconnectBar('进程退出码: ' + msg.exit_code, true);
                break;
            case 'pong':
                break;
            case 'session_status':
                if (msg.status === 'orphan') {
                    setStatus('disconnected', 'Mac 离线，等待重连...');
                    showReconnectBar('Agent 未连接，请确认 Mac 端正在运行', true);
                } else if (msg.status === 'active') {
                    setStatus('connected', '已恢复连接');
                    hideReconnectBar();
                }
                break;
        }
    }

    function sendInput(data) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            var encoder = new TextEncoder();
            ws.send(encoder.encode(data));
        }
    }

    function sendResize() {
        if (ws && ws.readyState === WebSocket.OPEN && term) {
            ws.send(JSON.stringify({
                type: 'resize',
                cols: term.cols,
                rows: term.rows
            }));
        }
    }

    function startPing() {
        stopPing();
        pingInterval = setInterval(function() {
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'ping', ts: Date.now() }));
            }
        }, 10000);
    }

    function stopPing() {
        if (pingInterval) {
            clearInterval(pingInterval);
            pingInterval = null;
        }
    }

    function setStatus(state, text) {
        statusBar.className = 'status-bar status-' + state;
        statusText.textContent = text;
    }

    /* ========== Reconnect ========== */

    function scheduleReconnect() {
        if (reconnecting || !sessionId) return;
        reconnecting = true;

        var delays = [1000, 2000, 5000];
        var delay = delays[Math.min(reconnectAttempts, delays.length - 1)];

        var attemptText = '重连中 (' + (reconnectAttempts + 1) + '/' + maxReconnectAttempts + ')...';
        setStatus('connecting', attemptText);
        showReconnectBar(lastDisconnectReason, false);

        var seq = connectSeq;
        reconnectTimer = setTimeout(function() {
            if (seq !== connectSeq) return;
            reconnectAttempts++;
            reconnecting = false;

            fetch('/api/agents', {
                headers: { 'Authorization': 'Bearer ' + jwtToken }
            }).then(function(resp) {
                if (seq !== connectSeq) return;
                if (resp.status === 401) {
                    redirectToLogin();
                    return;
                }
                if (reconnectAttempts > maxReconnectAttempts) {
                    setStatus('disconnected', '重连失败');
                    showReconnectBar(lastDisconnectReason || '多次重连未成功', true);
                    return;
                }
                connectWebSocket(function() {
                    if (seq !== connectSeq) return;
                    sessionId = null;
                    localStorage.removeItem('linkterm_session_id');
                    setStatus('disconnected', '会话已失效');
                    showReconnectBar('会话不存在，请重试或创建新终端', true);
                });
            }).catch(function() {
                if (seq !== connectSeq) return;
                if (reconnectAttempts > maxReconnectAttempts) {
                    setStatus('disconnected', '网络异常');
                    showReconnectBar('无法连接到服务端，请检查网络', true);
                } else {
                    scheduleReconnect();
                }
            });
        }, delay);
    }

    /* ========== Visibility (reconnect on resume) ========== */

    function setupVisibility() {
        document.addEventListener('visibilitychange', function() {
            if (document.visibilityState === 'hidden') {
                lastHiddenTime = Date.now();
            } else {
                var elapsed = Date.now() - lastHiddenTime;
                if (!ws || ws.readyState !== WebSocket.OPEN) {
                    cancelReconnect();
                    reconnectAttempts = 0;
                    reconnecting = false;
                    hideReconnectBar();
                    connectToSession();
                } else if (elapsed > 30000) {
                    try {
                        ws.send(JSON.stringify({ type: 'ping', ts: Date.now() }));
                    } catch (e) {}
                    var seqBeforePing = connectSeq;
                    setTimeout(function() {
                        if (seqBeforePing !== connectSeq) return;
                        if (!ws || ws.readyState !== WebSocket.OPEN) {
                            cancelReconnect();
                            reconnectAttempts = 0;
                            reconnecting = false;
                            connectToSession();
                        }
                    }, 3000);
                }
            }
        });
    }

    function cancelReconnect() {
        if (reconnectTimer) {
            clearTimeout(reconnectTimer);
            reconnectTimer = null;
        }
    }

    function setupReconnectActions() {
        document.getElementById('retryBtn').addEventListener('click', function() {
            connectSeq++;
            cancelReconnect();
            reconnectAttempts = 0;
            reconnecting = false;
            hideReconnectBar();
            sessionId = null;
            localStorage.removeItem('linkterm_session_id');
            closeWebSocket();
            connectToSession();
        });
        document.getElementById('reloginBtn').addEventListener('click', function() {
            redirectToLogin();
        });
    }

    /* ========== Toolbar ========== */

    function setupToolbar() {
        var buttons = document.querySelectorAll('.key-btn[data-key]');
        for (var i = 0; i < buttons.length; i++) {
            buttons[i].addEventListener('click', function(e) {
                var key = this.getAttribute('data-key');
                handleToolbarKey(key);
                e.preventDefault();
            });
        }

        var pasteBtn = document.getElementById('pasteBtn');
        if (pasteBtn) {
            pasteBtn.addEventListener('click', function(e) {
                e.preventDefault();
                if (navigator.clipboard && navigator.clipboard.readText) {
                    navigator.clipboard.readText().then(function(text) {
                        if (text) sendInput(text);
                        term.focus();
                    }).catch(function() {
                        term.focus();
                    });
                }
            });
        }

    }

    function applyFontPreset(preset) {
        if (!fontPresets[preset]) return;
        currentFontPreset = preset;
        localStorage.setItem('linkterm_fontpreset', preset);
        term.options.fontSize = fontPresets[preset].size;
        term.options.lineHeight = fontPresets[preset].line;
        fitAddon.fit();
        sendResize();
        updateFontBtnState();
        term.focus();
    }

    function updateFontBtnState() {
        var btns = document.querySelectorAll('.font-btn');
        for (var i = 0; i < btns.length; i++) {
            if (btns[i].getAttribute('data-fontsize') === currentFontPreset) {
                btns[i].classList.add('active');
            } else {
                btns[i].classList.remove('active');
            }
        }
    }

    function handleToolbarKey(key) {
        switch (key) {
            case 'Tab':
                sendInput('\t');
                break;
            case 'Escape':
                sendInput('\x1b');
                break;
            case 'Ctrl':
                ctrlActive = !ctrlActive;
                updateCtrlBtn();
                return;
            case 'ArrowUp':
                sendInput('\x1b[A');
                break;
            case 'ArrowDown':
                sendInput('\x1b[B');
                break;
            case 'ArrowRight':
                sendInput('\x1b[C');
                break;
            case 'ArrowLeft':
                sendInput('\x1b[D');
                break;
            default:
                sendInput(key);
        }
        term.focus();
    }

    function updateCtrlBtn() {
        var btn = document.querySelector('[data-key="Ctrl"]');
        if (btn) {
            if (ctrlActive) {
                btn.classList.add('active');
            } else {
                btn.classList.remove('active');
            }
        }
    }

    /* ========== Menu ========== */

    function setupMenu() {
        document.getElementById('menuBtn').addEventListener('click', function() {
            menuOverlay.classList.remove('hidden');
            loadSessionList();
        });
        document.getElementById('closeMenuBtn').addEventListener('click', function() {
            menuOverlay.classList.add('hidden');
        });
        document.getElementById('newTermBtn').addEventListener('click', function() {
            menuOverlay.classList.add('hidden');
            connectSeq++;
            cancelReconnect();
            closeWebSocket();
            sessionId = null;
            localStorage.removeItem('linkterm_session_id');
            term.reset();
            setStatus('connecting', '正在创建...');
            if (!agentId) {
                var seq = connectSeq;
                fetch('/api/agents', {
                    headers: { 'Authorization': 'Bearer ' + jwtToken }
                })
                .then(function(resp) {
                    if (seq !== connectSeq) return;
                    if (resp.status === 401) { redirectToLogin(); return; }
                    return resp.json();
                })
                .then(function(agents) {
                    if (seq !== connectSeq) return;
                    if (!agents || agents.length === 0) {
                        setStatus('disconnected', 'Mac 离线');
                        showReconnectBar('请确认 Agent 已启动并连接到服务端', true);
                        return;
                    }
                    agentId = agents[0].id;
                    createSession(agentId);
                })
                .catch(function(err) {
                    if (seq !== connectSeq) return;
                    setStatus('disconnected', '创建失败: ' + err.message);
                });
            } else {
                createSession(agentId);
            }
        });
        document.getElementById('closeTermBtn').addEventListener('click', function() {
            if (confirm('确认关闭终端？正在运行的进程将被终止。')) {
                menuOverlay.classList.add('hidden');
                connectSeq++;
                cancelReconnect();
                if (sessionId) {
                    fetch('/api/sessions?id=' + sessionId, {
                        method: 'DELETE',
                        headers: { 'Authorization': 'Bearer ' + jwtToken }
                    });
                }
                sessionId = null;
                localStorage.removeItem('linkterm_session_id');
                setStatus('disconnected', '终端已关闭');
                closeWebSocket();
            }
        });
        var fontBtns = document.querySelectorAll('.font-btn');
        for (var i = 0; i < fontBtns.length; i++) {
            fontBtns[i].addEventListener('click', function(e) {
                e.preventDefault();
                var preset = this.getAttribute('data-fontsize');
                applyFontPreset(preset);
            });
        }
        updateFontBtnState();

        document.getElementById('logoutBtn').addEventListener('click', function() {
            localStorage.removeItem('linkterm_token');
            localStorage.removeItem('linkterm_session_id');
            window.location.href = '/';
        });
        menuOverlay.addEventListener('click', function(e) {
            if (e.target === menuOverlay) {
                menuOverlay.classList.add('hidden');
            }
        });
    }

    /** loadSessionList 在菜单中加载可切换的会话列表 */
    function loadSessionList() {
        var listEl = document.getElementById('sessionList');
        if (!listEl) return;
        listEl.innerHTML = '<div style="padding:8px 16px;color:#565f89;">加载中...</div>';

        fetch('/api/sessions', {
            headers: { 'Authorization': 'Bearer ' + jwtToken }
        })
        .then(function(resp) {
            if (resp.status === 401) { redirectToLogin(); return; }
            return resp.json();
        })
        .then(function(sessions) {
            if (!sessions) return;
            listEl.innerHTML = '';
            if (sessions.length === 0) {
                listEl.innerHTML = '<div style="padding:8px 16px;color:#565f89;">无活跃会话</div>';
                return;
            }
            for (var i = 0; i < sessions.length; i++) {
                var s = sessions[i];
                var item = document.createElement('div');
                item.className = 'session-item' + (s.id === sessionId ? ' active' : '');
                item.setAttribute('data-id', s.id);

                var statusClass = 'status-dot status-' + s.status;
                var statusLabel = { active: '活跃', detached: '分离', orphan: '离线' }[s.status] || s.status;

                item.innerHTML = '<span class="' + statusClass + '"></span>' +
                    '<span class="session-name">' + s.id.substring(0, 16) + '</span>' +
                    '<span class="session-status">' + statusLabel + '</span>';

                item.addEventListener('click', (function(sid) {
                    return function() {
                        menuOverlay.classList.add('hidden');
                        if (sid !== sessionId) {
                            connectSeq++;
                            cancelReconnect();
                            closeWebSocket();
                            sessionId = sid;
                            localStorage.setItem('linkterm_session_id', sid);
                            term.clear();
                            connectWebSocket(function() {
                                sessionId = null;
                                localStorage.removeItem('linkterm_session_id');
                                connectToSession();
                            });
                        }
                    };
                })(s.id));

                listEl.appendChild(item);
            }
        })
        .catch(function() {
            listEl.innerHTML = '<div style="padding:8px 16px;color:#f7768e;">加载失败</div>';
        });
    }

    function formatCloseReason(code, reason) {
        if (reason) return reason;
        var reasons = {
            1000: '正常关闭',
            1001: '页面离开',
            1006: '连接异常断开（网络中断或服务端无响应）',
            1011: '服务端内部错误',
            1012: '服务端重启',
            1013: '服务端过载'
        };
        return reasons[code] || '连接关闭 (code: ' + code + ')';
    }

    function formatBytes(bytes) {
        if (bytes < 1024) return bytes + ' B';
        return (bytes / 1024).toFixed(1) + ' KB';
    }
})();
