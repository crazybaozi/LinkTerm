package main

import (
	"fmt"
	"strings"
)

/** PrintAccessInfo 在终端打印访问地址，方便手机扫码 */
func PrintAccessInfo(serverURL string, serverName string) {
	accessURL := serverToHTTP(serverURL)

	fmt.Println()
	fmt.Println("  ┌───────────────────────────────────────┐")
	fmt.Println("  │         LinkTerm Agent Running         │")
	fmt.Println("  ├───────────────────────────────────────┤")
	fmt.Printf("  │  Server : %-28s│\n", serverName)
	fmt.Printf("  │  URL    : %-28s│\n", truncate(accessURL, 28))
	fmt.Println("  ├───────────────────────────────────────┤")
	fmt.Println("  │  手机浏览器打开上面的 URL 即可使用      │")
	fmt.Println("  │  Press Ctrl+C to stop                 │")
	fmt.Println("  └───────────────────────────────────────┘")
	fmt.Println()
}

func serverToHTTP(wsURL string) string {
	url := wsURL
	url = strings.Replace(url, "wss://", "https://", 1)
	url = strings.Replace(url, "ws://", "http://", 1)
	return url
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
