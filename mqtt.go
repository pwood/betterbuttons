package main

import (
	"encoding/json"
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

type MQTT struct {
	client        mqtt.Client
	buttonmanager Manager
	logger        *slog.Logger
}

func (m *MQTT) Start(mqttUrl string) error {
	opts := mqtt.NewClientOptions()
	opts.ClientID = "BetterButtons"

	if u, err := url.Parse(mqttUrl); err != nil {
		return fmt.Errorf("mqtt url parse: %w", err)
	} else {
		opts.Servers = []*url.URL{u}
	}

	opts.SetOnConnectHandler(m.connected)
	opts.SetConnectionLostHandler(m.disconnected)

	m.client = mqtt.NewClient(opts)

	retry := time.NewTicker(1 * time.Second)

	m.logger.Info("Attempting to connect to MQTT.")

	for {
		select {
		case <-retry.C:
			token := m.client.Connect()
			token.Wait()

			if err := token.Error(); err != nil {
				m.logger.Error("Connect attempt failed, will retry.", "err", err)
			} else {
				return nil
			}
		}
	}
}

func (m *MQTT) Stop() error {
	m.logger.Info("Disconnecting from MQTT.")
	m.client.Disconnect(1500)
	return nil
}

func (m *MQTT) connected(c mqtt.Client) {
	m.logger.Info("Connected to MQTT.")
	c.Subscribe("zigbee2mqtt/bridge/devices", 1, m.messageDeviceList)
}

func (m *MQTT) disconnected(c mqtt.Client, err error) {
	m.logger.Error("Disconnected from MQTT!", "err", err)
	panic("MQTT gone.")
}

func (m *MQTT) messageAction(c mqtt.Client, message mqtt.Message) {
	u := DeviceUpdate{IEEEAddress: strings.TrimPrefix(message.Topic(), "zigbee2mqtt/")}

	if err := json.Unmarshal(message.Payload(), &u); err != nil {
		m.logger.Error("Unable to unmarshal action payload.", "err", err)
		return
	}

	m.buttonmanager.Update(u)
}

func (m *MQTT) messageDeviceList(c mqtt.Client, message mqtt.Message) {
	dl := DeviceList{}

	if err := json.Unmarshal(message.Payload(), &dl); err != nil {
		m.logger.Error("Unable to unmarshal device list payload.", "err", err)
		return
	}

	for _, d := range dl {
		if m.buttonmanager.OfferDevice(d) {
			topic := fmt.Sprintf("zigbee2mqtt/%s", d.IEEEAddress)
			token := c.Subscribe(topic, 1, m.messageAction)

			if !token.WaitTimeout(5 * time.Second) {
				m.logger.Error("Failed to subscribe to action for device.", "err", token.Error(), "topic", topic)
			} else {
				m.logger.Info("Successfully subscribed to device actions.", "id", d.IEEEAddress)
			}
		}
	}

	m.buttonmanager.UpdateHomeKit()

	// TODO: Handle removed
}
