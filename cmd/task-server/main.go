package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"task-manager/internal/config"
	"task-manager/internal/middleware"
	"task-manager/internal/auth"
	"task-manager/internal/reviews"
	"task-manager/internal/events"
	"task-manager/internal/rooms"
	"task-manager/internal/teachers"
	"task-manager/internal/qa"
	"task-manager/internal/notify"
	"task-manager/internal/users"

	sqlstore "task-manager/internal/sql"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		log.Fatalf("DATABASE_URL is required")
	}

	// 1) Миграции — до всего остального.
	if err := sqlstore.RunUp(sqlstore.Config{
		DSN:             cfg.DatabaseURL,
		Timeout:         cfg.MigrateTimeout,
		MigrationsTable: cfg.MigrationsTable,
	}); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// 2) Пул соединений. Один на приложение.
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		pingCancel()
		log.Fatalf("db ping: %v", err)
	}
	pingCancel()

	// 3) Корневой контекст приложения — kill switch для фоновых задач сервиса
	//    и для BaseContext (r.Context()). Отменяется ПОСЛЕ Shutdown, через defer.
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// 4) Контекст сигналов.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 5) Слои.

	//auth
	authStore := auth.NewStore(db)
	authSvc := auth.NewService(authStore, auth.Config{
		JWTSecret:  []byte(cfg.JWTSecret),
		Issuer:     "task-manager",
		AccessTTL:  cfg.AccessTTL,
		RefreshTTL: cfg.RefreshTTL,
	})
	authHandler := auth.NewHandler(authSvc)

	// Notify
	notifyStore := notify.NewStore(db)
	notifySvc := notify.NewService(notifyStore)
	notifyHandler := notify.NewHandler(notifySvc, authSvc)
	
	//reviews
	reviewsStore := reviews.NewStore(db)
	reviewsSvc := reviews.NewService(reviewsStore, notify.NewReviewsAdapter(notifySvc)) 
	reviewsHandler := reviews.NewHandler(reviewsSvc, authSvc)
	// фоновый воркер: раз в час переводит "new" старше 5 дней в "in_progress"
	go reviewsSvc.RunStaleWatcher(appCtx, time.Hour, 5*24*time.Hour)

	//events
	eventsStore := events.NewStore(db)
	eventsSvc := events.NewService(eventsStore)
	eventsHandler := events.NewHandler(eventsSvc, authSvc)

	//rooms
	roomsStore := rooms.NewStore(db)
	roomsSvc := rooms.NewService(roomsStore)
	roomsHandler := rooms.NewHandler(roomsSvc, authSvc)

	//teachers
	teachersStore := teachers.NewStore(db)
	teachersSvc := teachers.NewService(teachersStore)
	teachersHandler := teachers.NewHandler(teachersSvc, authSvc)

	//qa
	qaNotifier := notify.NewQAAdapter(notifySvc) 
	qaStore := qa.NewStore(db)
	qaSvc := qa.NewService(qaStore, qaNotifier)
	qaHandler := qa.NewHandler(qaSvc, authSvc)

	//users
	usersStore := users.NewStore(db)
	usersSvc := users.NewService(usersStore)
	usersHandler := users.NewHandler(usersSvc, authSvc)

	r := buildRouter(
		authHandler, authSvc, 
		reviewsHandler,eventsHandler,
		roomsHandler,teachersHandler,
		qaHandler,notifyHandler,
		usersHandler,
	)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,

		BaseContext: func(net.Listener) context.Context { return appCtx },
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	log.Printf("server starting addr=%s db=%s", srv.Addr, redactDSN(cfg.DatabaseURL))

	serverErrCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case <-sigCtx.Done():
		log.Printf("shutdown signal received")
	case err := <-serverErrCh:
		if err == nil {
			// Сервер сам завершился без ошибки — выходим.
			// appCancel отработает через defer.
			return
		}
		log.Printf("server error: %v", err)
	}

	// 6) КЛАССИЧЕСКИЙ graceful shutdown:
	//    сначала перестаём принимать новые запросы и ждём in-flight,
	//    appCtx ещё жив → requests дорабатывают штатно.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		_ = srv.Close() // добиваем жёстко
	}

	// 7) appCancel() явно не нужен — он defer’нут первым, сработает при return.
	//    Если хочешь явности — можно продублировать: context.WithCancel идемпотентен.
	log.Printf("server stopped")
}

func buildRouter(
	authHandler 		*auth.Handler,
	authSvc 			*auth.Service,
	reviewsHandler 		*reviews.Handler,
	eventsHandler		*events.Handler,
	roomsHandler 		*rooms.Handler,
	teachersHandler 	*teachers.Handler,
	qaHandler 			*qa.Handler,
	notifyHandler		*notify.Handler,
	usersHandler 		*users.Handler,
	) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestIDMiddleware)
	r.Use(chiMiddleware.Recoverer)

	r.Mount("/api/v1/auth", authHandler.Router())

	r.With(middleware.AuthMiddleware(authSvc)).
		Get("/api/v1/auth/me", authHandler.MeHandler())

	r.Mount("/api/v1/reviews", reviewsHandler.Router())

	r.Mount("/api/v1/events", eventsHandler.Router()) 

	r.Mount("/api/v1/rooms", roomsHandler.Router())

	r.Mount("/api/v1/teachers", teachersHandler.Router())

	r.Mount("/api/v1/qa", qaHandler.Router()) 

	r.Mount("/api/v1/notifications", notifyHandler.Router())

	r.Mount("/api/v1/users", usersHandler.Router())

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		middleware.WriteError(w, req, http.StatusNotFound, "not_found", "Route not found",
			map[string]any{"path": req.URL.Path})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		middleware.WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed",
			"Method not allowed", map[string]any{"method": req.Method, "path": req.URL.Path})
	})

	return r
}

// redactDSN прячет пароль из DSN в логах.
// "postgres://user:secret@host/db" → "postgres://user:***@host/db"
func redactDSN(dsn string) string {
	if i := strings.Index(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			if colon := strings.Index(rest[:at], ":"); colon >= 0 {
				return dsn[:i+3] + rest[:colon] + ":***" + rest[at:]
			}
		}
	}
	return dsn
}