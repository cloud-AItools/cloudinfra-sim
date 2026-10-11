// cloudinfra-sim은 CMP-Oriented VMware SDDC API Simulator의 단일 실행파일이다(권고안 A).
//
//	cloudinfra-sim --vcenter-version 7.0.3.02500 --nsx-version 3.2.4.3
//	cloudinfra-sim cert rotate --domain cloudinfra-sim.test   # 서버 인증서 교체(NFR-0303)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"cloudinfra-sim/internal/certs"
	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/sim"
	"cloudinfra-sim/internal/tempdir"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) >= 2 && args[0] == "cert" && args[1] == "rotate" {
		return rotate(args[2:])
	}
	if len(args) >= 1 && args[0] == "version" {
		fmt.Printf("cloudinfra-sim build=%s commit=%s\n", config.BuildKind, sim.BuildCommit)
		return 0
	}
	cfg, err := config.Parse(args, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "설정 오류:", err)
		return 2
	}
	log := newLogger(cfg)
	// 임시 파일(vcsim 데이터스토어 govcsim-* 등)을 <data-dir>/tmp/<pid>에 모은다. 정상 종료 때 지우고, 작업 관리자
	// 종료·kill -9 등으로 남은 이전 실행의 것은 여기서 지운다(실행 중인 pid의 것은 남긴다).
	if _, swept, cleanup, err := tempdir.Isolate(filepath.Join(cfg.DataDir, "tmp")); err != nil {
		log.Warn("전용 임시 디렉터리를 만들지 못해 OS 임시 디렉터리를 쓴다", "err", err)
	} else {
		defer cleanup()
		if len(swept) > 0 {
			log.Info("이전 실행이 강제 종료로 남긴 임시 디렉터리를 지웠다", "count", len(swept))
		}
	}
	s, err := sim.New(cfg, log)
	if err != nil {
		log.Error("기동 거부", "err", err.Error())
		return 1
	}
	if err := s.Start(); err != nil {
		log.Error("기동 실패", "err", err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Shutdown(shutdown)
	log.Info("cloudinfra-sim 종료")
	return 0
}

func rotate(args []string) int {
	cfg, err := config.Parse(append(args, "--vcenter-version", "-", "--nsx-version", "-"), os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "설정 오류:", err)
		return 2
	}
	b, err := certs.Rotate(filepath.Join(cfg.DataDir, "certs"), cfg.Domain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "인증서 교체 실패:", err)
		return 1
	}
	fmt.Printf("서버 인증서를 교체했다. 새 지문(SHA-256): %s\n", b.ThumbprintSHA256())
	return 0
}

// newLogger는 사람이 읽는 형식과 JSON 형식을 지원한다(NFR-0405).
func newLogger(cfg *config.Config) *slog.Logger {
	lvl := new(slog.LevelVar)
	_ = lvl.UnmarshalText([]byte(cfg.LogLevel))
	opts := &slog.HandlerOptions{Level: lvl}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
