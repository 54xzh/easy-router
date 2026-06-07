package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"easy-router/internal/admin"
	"easy-router/internal/config"
	"easy-router/internal/proxy"
	"easy-router/internal/store"
)

//go:embed web/dist
var webDist embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("启动失败：%v", err)
	}

	db, err := store.Open(cfg.DBPath, cfg.SecretKey)
	if err != nil {
		log.Fatalf("打开数据库失败：%v", err)
	}
	defer db.Close()

	initialPassword, err := db.EnsureAdminUser()
	if err != nil {
		log.Fatalf("初始化管理员失败：%v", err)
	}
	if initialPassword != "" {
		log.Printf("首次启动管理员账号：admin")
		log.Printf("首次启动管理员密码：%s", initialPassword)
		log.Printf("请登录后尽快修改密码。")
	}

	mux := http.NewServeMux()
	admin.Register(mux, db)
	proxy.Register(mux, db)
	registerStatic(mux)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           withRequestLimits(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Easy Router 已启动：http://%s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务异常退出：%v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func registerStatic(mux *http.ServeMux) {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		panic(fmt.Sprintf("前端文件缺失：%v", err))
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			serveIndex(w, r, sub)
			return
		}
		if _, err := fs.Stat(sub, r.URL.Path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, sub)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	payload, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "前端文件未构建，请先运行 npm run build", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func withRequestLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		next.ServeHTTP(w, r)
	})
}
