package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	mqttURL := flag.String("mqtt-url", "mqtt://localhost:1883", "URL of MQTT server to connect to.")
	homekitDir := flag.String("homekit-dir", "./db", "Location on disk to store HomeKit pairing state.")
	debug := flag.Bool("debug", false, "Debug logging.")

	flag.Parse()

	lvl := &slog.HandlerOptions{Level: slog.LevelInfo}

	if *debug {
		lvl.Level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, lvl))
	logger.Info("Starting BetterButtons", "mqttURL", *mqttURL, "homekitDir", *homekitDir, "debug", *debug)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	h := &HomeKit{logger: logger}
	b := Manager{logger: logger, Devices: map[string]*ButtonDevice{}, HKManager: h}
	m := MQTT{logger: logger, buttonmanager: b}

	h.Init(ctx, *homekitDir)
	go b.Run(ctx)

	if err := m.Start(ctx, *mqttURL); err != nil {
		logger.Error("Failed to start MQTT client.", "err", err)
		panic(err)
	}

	select {
	case <-ctx.Done():
		stop()
	}

	logger.Info("Stopping program.")

	if err := m.Stop(); err != nil {
		logger.Error("Failed to stop MQTT client.", "err", err)
		panic(err)
	}
}
