package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"trippilot/internal/agent"
	"trippilot/internal/api"
	"trippilot/internal/config"
	"trippilot/internal/eval"
	"trippilot/internal/events"
	"trippilot/internal/logging"
	"trippilot/internal/repo"
	"trippilot/internal/runstore"
	"trippilot/internal/tools"
	"trippilot/migrations"
)

const version = "0.1.0-dev"

const poiSnapshotPath = "data/poi/beijing.json"
const evalCasesPath = "eval/cases"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	logger.Info("trippilot api starting", "version", version, "port", cfg.Port)

	if cfg.LLMBaseURL == "" || cfg.LLMModel == "" {
		return errors.New("LLM_BASE_URL and LLM_MODEL are required")
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()
	if err := db.PingContext(startupCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := migrations.Up(db); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	store, err := repo.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	logger.Info("database ready")

	snapshot, err := tools.LoadSnapshot(poiSnapshotPath)
	if err != nil {
		return err
	}

	llm, err := agent.NewOpenAIClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel,
		agent.WithLLMLogger(logger))
	if err != nil {
		return err
	}

	var amap *tools.AMapClient
	if cfg.AMapWebKey != "" {
		amap = tools.NewAMapClient(cfg.AMapWebKey)
	}
	nodeOptions := []agent.NodeOption{
		agent.WithSnapshotSource(snapshot),
		agent.WithPOILookup(snapshot),
		agent.WithRouteProvider(tools.NewRouteService(amap, "beijing")),
		agent.WithNodeLogger(logger),
	}
	if amap != nil {
		nodeOptions = append(nodeOptions, agent.WithExternalSource(tools.NewAMapSource(amap)))
	}

	broker, err := events.NewBroker(events.NewRepoStore(store), events.WithLogger(logger))
	if err != nil {
		return err
	}
	defer broker.Close()

	nodes := agent.NewNodes(llm, broker, nodeOptions...)
	runner, err := agent.NewRunner(startupCtx, runstore.New(store), broker, nodes,
		agent.WithRunnerConcurrency(3),
		agent.WithRunnerLogger(logger),
	)
	if err != nil {
		return err
	}
	recoverCtx, cancelRecover := context.WithTimeout(context.Background(), 30*time.Second)
	if err := runner.Recover(recoverCtx); err != nil {
		logger.Error("recover runs", "error", err)
	}
	cancelRecover()

	router := api.NewRouter(api.Deps{
		Version:          version,
		Logger:           logger,
		AdminToken:       cfg.AdminToken,
		RateLimitPerHour: cfg.RateLimitPerHour,
		Ready:            store.Ping,
		Broker:           broker,
		Store:            store,
		Runner:           runner,
		Eval:             newEvalService(store, runner, logger),
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "error", err)
	}
	return nil
}

func newEvalService(store *repo.Store, runner *agent.Runner, logger *slog.Logger) api.EvalService {
	cases, err := eval.LoadCases(evalCasesPath)
	if err != nil {
		logger.Warn("eval cases unavailable", "error", err)
		return nil
	}
	batch := eval.NewRunner(store, runner, eval.WithRunnerLogger(logger))
	service, err := eval.NewService(store, batch, cases, logger)
	if err != nil {
		logger.Warn("eval service unavailable", "error", err)
		return nil
	}
	return service
}
