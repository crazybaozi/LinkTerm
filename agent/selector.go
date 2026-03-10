package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

/** NodeResult 单节点测速结果 */
type NodeResult struct {
	Node    ServerNode
	Latency time.Duration
	OK      bool
}

/** Selector 智能选路：多节点测速、选择最优、定期检测、自动切换 */
type Selector struct {
	nodes          []ServerNode
	tunnel         *Tunnel
	currentIdx     int
	mu             sync.Mutex
	stopCh         chan struct{}
	switchCount    int
	checkInterval  time.Duration
	pingTimeout    time.Duration
	pingCount      int
}

func NewSelector(nodes []ServerNode, tunnel *Tunnel) *Selector {
	return &Selector{
		nodes:         nodes,
		tunnel:        tunnel,
		currentIdx:    -1,
		stopCh:        make(chan struct{}),
		checkInterval: 5 * time.Minute,
		pingTimeout:   3 * time.Second,
		pingCount:     3,
	}
}

/** SelectBest 并发测速所有节点，返回最优节点的索引 */
func (s *Selector) SelectBest() (int, []NodeResult) {
	results := s.PingAll()
	if len(results) == 0 {
		return -1, results
	}

	var available []struct {
		idx    int
		result NodeResult
	}
	for i, r := range results {
		if r.OK {
			available = append(available, struct {
				idx    int
				result NodeResult
			}{i, r})
		}
	}

	if len(available) == 0 {
		return -1, results
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].result.Latency < available[j].result.Latency
	})

	return available[0].idx, results
}

/** PingAll 并发 ping 所有节点 */
func (s *Selector) PingAll() []NodeResult {
	results := make([]NodeResult, len(s.nodes))
	var wg sync.WaitGroup

	for i, node := range s.nodes {
		wg.Add(1)
		go func(idx int, n ServerNode) {
			defer wg.Done()
			latency, ok := s.pingNode(n)
			results[idx] = NodeResult{
				Node:    n,
				Latency: latency,
				OK:      ok,
			}
		}(i, node)
	}

	wg.Wait()
	return results
}

/** pingNode 对单个节点做 N 次 ping，返回中位数延迟 */
func (s *Selector) pingNode(node ServerNode) (time.Duration, bool) {
	url := nodeToHTTP(node.URL) + "/health/ping"
	client := &http.Client{Timeout: s.pingTimeout}

	var latencies []time.Duration
	for i := 0; i < s.pingCount; i++ {
		start := time.Now()
		resp, err := client.Get(url)
		elapsed := time.Since(start)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			latencies = append(latencies, elapsed)
		}
	}

	if len(latencies) == 0 {
		return 0, false
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	median := latencies[len(latencies)/2]
	return median, true
}

/** ConnectBest 测速并连接最优节点 */
func (s *Selector) ConnectBest() error {
	log.Println("[selector] measuring latency to all nodes...")

	bestIdx, results := s.SelectBest()
	for _, r := range results {
		if r.OK {
			log.Printf("[selector]   %s: %v", r.Node.Name, r.Latency.Round(time.Millisecond))
		} else {
			log.Printf("[selector]   %s: unreachable", r.Node.Name)
		}
	}

	if bestIdx < 0 {
		return fmt.Errorf("all nodes unreachable")
	}

	best := s.nodes[bestIdx]
	log.Printf("[selector] selected: %s (%v)", best.Name, results[bestIdx].Latency.Round(time.Millisecond))

	s.mu.Lock()
	s.currentIdx = bestIdx
	s.mu.Unlock()

	return s.tunnel.Connect(best.URL, best.Name)
}

/** ReconnectBest 断线重连：先快试当前节点，失败则测速选路 */
func (s *Selector) ReconnectBest() {
	s.tunnel.setStatus(StatusReconnecting, "reconnecting...")

	s.mu.Lock()
	currentIdx := s.currentIdx
	s.mu.Unlock()

	if currentIdx >= 0 && currentIdx < len(s.nodes) {
		node := s.nodes[currentIdx]
		log.Printf("[selector] quick retry current node: %s", node.Name)
		if err := s.tunnel.Connect(node.URL, node.Name); err == nil {
			return
		}
	}

	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	maxInterval := time.Duration(s.tunnel.config.ReconnectMaxInterval) * time.Second
	if maxInterval == 0 {
		maxInterval = 30 * time.Second
	}

	for attempt := 0; ; attempt++ {
		delay := backoffs[min(attempt, len(backoffs)-1)]
		if delay > maxInterval {
			delay = maxInterval
		}
		log.Printf("[selector] reconnect via select-best in %v (attempt %d)", delay, attempt+1)
		time.Sleep(delay)

		if err := s.ConnectBest(); err == nil {
			return
		}
	}
}

/** StartMonitor 启动后台定期检测，发现更优节点时自动切换 */
func (s *Selector) StartMonitor() {
	if len(s.nodes) <= 1 {
		return
	}
	go s.monitorLoop()
}

func (s *Selector) monitorLoop() {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	consecutiveBetter := 0

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.tunnel.Status() != StatusConnected {
				consecutiveBetter = 0
				continue
			}

			results := s.PingAll()

			s.mu.Lock()
			curIdx := s.currentIdx
			s.mu.Unlock()

			if curIdx < 0 || curIdx >= len(results) || !results[curIdx].OK {
				consecutiveBetter = 0
				continue
			}

			curLatency := results[curIdx].Latency

			bestIdx := -1
			var bestLatency time.Duration
			for i, r := range results {
				if i == curIdx || !r.OK {
					continue
				}
				if bestIdx < 0 || r.Latency < bestLatency {
					bestIdx = i
					bestLatency = r.Latency
				}
			}

			if bestIdx < 0 {
				consecutiveBetter = 0
				continue
			}

			diff := float64(curLatency-bestLatency) / float64(curLatency)
			if bestLatency*2 < curLatency && diff > 0.2 {
				consecutiveBetter++
				log.Printf("[selector] better node detected: %s (%v vs current %v), streak=%d/3",
					s.nodes[bestIdx].Name, bestLatency.Round(time.Millisecond), curLatency.Round(time.Millisecond), consecutiveBetter)
			} else {
				consecutiveBetter = 0
			}

			if consecutiveBetter >= 3 {
				consecutiveBetter = 0
				s.switchToNode(bestIdx)
			}
		}
	}
}

func (s *Selector) switchToNode(newIdx int) {
	node := s.nodes[newIdx]
	log.Printf("[selector] switching to better node: %s", node.Name)

	s.tunnel.intentional = true
	err := s.tunnel.Connect(node.URL, node.Name)
	if err != nil {
		log.Printf("[selector] switch failed, keeping current connection: %v", err)
		s.tunnel.intentional = false
		return
	}

	s.mu.Lock()
	s.currentIdx = newIdx
	s.switchCount++
	s.mu.Unlock()

	log.Printf("[selector] switched to %s successfully (total switches: %d)", node.Name, s.switchCount)
}

/** Stop 停止后台检测 */
func (s *Selector) Stop() {
	close(s.stopCh)
}

/** CurrentNode 返回当前连接的节点 */
func (s *Selector) CurrentNode() *ServerNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentIdx >= 0 && s.currentIdx < len(s.nodes) {
		n := s.nodes[s.currentIdx]
		return &n
	}
	return nil
}

func nodeToHTTP(wsURL string) string {
	url := wsURL
	url = strings.Replace(url, "wss://", "https://", 1)
	url = strings.Replace(url, "ws://", "http://", 1)
	return url
}
