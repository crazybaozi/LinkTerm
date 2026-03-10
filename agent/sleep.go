package main

import (
	"log"
	"os/exec"
	"runtime"
)

/** SleepGuard 通过 caffeinate 防止 Mac 进入空闲睡眠 */
type SleepGuard struct {
	cmd *exec.Cmd
}

/** NewSleepGuard 创建并启动防睡眠进程（仅 macOS 有效） */
func NewSleepGuard() *SleepGuard {
	if runtime.GOOS != "darwin" {
		return &SleepGuard{}
	}

	cmd := exec.Command("caffeinate", "-di")
	if err := cmd.Start(); err != nil {
		log.Printf("[sleep-guard] failed to start caffeinate: %v", err)
		return &SleepGuard{}
	}

	log.Printf("[sleep-guard] caffeinate started (pid=%d), idle sleep prevented", cmd.Process.Pid)
	return &SleepGuard{cmd: cmd}
}

/** Stop 停止防睡眠 */
func (s *SleepGuard) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
		log.Println("[sleep-guard] caffeinate stopped")
	}
}
