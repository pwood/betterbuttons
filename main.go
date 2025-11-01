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
	mqttTopicPrefix := flag.String("mqtt-topic-prefix", "zigbee2mqtt", "MQTT topic prefix to subscribe to.")

	flag.Parse()

	lvl := &slog.HandlerOptions{Level: slog.LevelInfo}

	if *debug {
		lvl.Level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, lvl))
	logger.Info("Starting BetterButtons", "mqttURL", *mqttURL, "mqttPrefix", mqttTopicPrefix, "homekitDir", *homekitDir, "debug", *debug)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	h := &HomeKit{logger: logger}
	b := Manager{logger: logger, Devices: map[string]*ButtonDevice{}, HKManager: h}
	m := MQTT{logger: logger, buttonmanager: b, topicPrefix: *mqttTopicPrefix}

	if err := h.Init(ctx, *homekitDir); err != nil {
		logger.Error("Failed to init HomeKit.", "err", err)
		panic(err)
	}

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
