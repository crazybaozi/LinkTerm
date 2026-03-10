(function() {
    var token = localStorage.getItem('linkterm_token');
    if (token) {
        window.location.href = '/terminal.html';
        return;
    }

    var form = document.getElementById('loginForm');
    var tokenInput = document.getElementById('agentToken');
    var loginBtn = document.getElementById('loginBtn');
    var errorEl = document.getElementById('loginError');

    form.addEventListener('submit', function(e) {
        e.preventDefault();
        errorEl.classList.add('hidden');
        loginBtn.textContent = '连接中...';
        loginBtn.disabled = true;

        fetch('/api/auth', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ token: tokenInput.value })
        })
        .then(function(resp) {
            if (!resp.ok) {
                return resp.json().then(function(data) {
                    throw new Error(data.error || '认证失败');
                });
            }
            return resp.json();
        })
        .then(function(data) {
            localStorage.setItem('linkterm_token', data.token);
            window.location.href = '/terminal.html';
        })
        .catch(function(err) {
            errorEl.textContent = err.message || '连接失败，请重试';
            errorEl.classList.remove('hidden');
            loginBtn.textContent = '连接';
            loginBtn.disabled = false;
            tokenInput.focus();
        });
    });

    tokenInput.focus();
})();
