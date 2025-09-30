package main

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
)

type Button struct {
	Name string

	HKSwitch *service.StatelessProgrammableSwitch

	SupportsSingle bool
	SupportsDouble bool
	SupportsLong   bool

	PressChain int
	LastAction ButtonAction
	ActionTime time.Time
}

type ButtonDevice struct {
	IEEEAddress string

	Manufacturer    string
	Model           string
	SoftwareBuildID string

	SupportsBattery bool
	HKBattery       *service.BatteryService

	SynthesiseButtons bool
	Buttons           []*Button

	MappingFunction ActionMapper
}

type ActionMapper func(DeviceUpdate) (int, ButtonAction)

type Manager struct {
	Devices   map[string]*ButtonDevice
	HKManager *HomeKit
	logger    *slog.Logger

	DevicesDirty bool
}

func (m *Manager) OfferDevice(d Device) bool {
	if _, found := m.Devices[d.IEEEAddress]; found {
		return false
	}

	m.logger.Info("Offered unregistered device.", "id", d.IEEEAddress, "manufacturer", d.Manufacturer, "model", d.ModelID)

	if mk, up, ok := lookupRegistry(d); ok {
		bd := &ButtonDevice{
			IEEEAddress:     d.IEEEAddress,
			Manufacturer:    d.Manufacturer,
			Model:           d.ModelID,
			SoftwareBuildID: d.SoftwareBuildID,
		}

		mk(bd)
		bd.MappingFunction = up
		m.Devices[d.IEEEAddress] = bd

		m.logger.Info("Accepted new device.", "id", d.IEEEAddress, "manufacturer", d.Manufacturer, "model", d.ModelID, "buttonCount", len(bd.Buttons))

		m.DevicesDirty = true

		return true
	}

	return false
}

func lookupRegistry(d Device) (func(*ButtonDevice), ActionMapper, bool) {
	switch d.Manufacturer {
	case "Philips":
		switch d.ModelID {
		case "RWL021":
			return makePhilipsRWL02X, mappingPhilipsRWL021, true
		}
	case "Signify Netherlands B.V.":
		switch d.ModelID {
		case "RWL022":
			return makePhilipsRWL02X, mappingPhilipsRWL021, true
		}
	case "eWeLink":
		switch d.ModelID {
		case "SNZB-01P":
			return makeeWeLinkSNZB01P, mappingSimple, true
		}
	case "IKEA of Sweden":
		switch d.ModelID {
		case "TRADFRI open/close remote":
			return makeIKEAE1766, mappingDouble, true
		}
	}

	return nil, nil, false
}

func makeeWeLinkSNZB01P(bd *ButtonDevice) {
	bd.SupportsBattery = true
	bd.SynthesiseButtons = false

	bd.Buttons = []*Button{
		{
			Name:           "Button",
			SupportsSingle: true,
			SupportsDouble: true,
			SupportsLong:   true,
		},
	}
}

func makeIKEAE1766(bd *ButtonDevice) {
	bd.SupportsBattery = true
	bd.SynthesiseButtons = false

	bd.Buttons = []*Button{
		{
			Name:           "Open",
			SupportsSingle: true,
			SupportsDouble: false,
			SupportsLong:   false,
		},
		{
			Name:           "Close",
			SupportsSingle: true,
			SupportsDouble: false,
			SupportsLong:   false,
		},
	}
}

func makePhilipsRWL02X(bd *ButtonDevice) {
	bd.SupportsBattery = true
	bd.SynthesiseButtons = false

	bd.Buttons = []*Button{
		{
			Name:           "On",
			SupportsSingle: true,
			SupportsDouble: false,
			SupportsLong:   true,
		},
		{
			Name:           "Up",
			SupportsSingle: true,
			SupportsDouble: false,
			SupportsLong:   true,
		},
		{
			Name:           "Down",
			SupportsSingle: true,
			SupportsDouble: false,
			SupportsLong:   true,
		},
		{
			Name:           "Off",
			SupportsSingle: true,
			SupportsDouble: false,
			SupportsLong:   true,
		},
	}
}

type ButtonAction int

const (
	None ButtonAction = iota
	Single
	Double
	Long
	Press
	Held
	Release
)

func mappingPhilipsRWL021(update DeviceUpdate) (int, ButtonAction) {
	actionParts := strings.SplitN(update.Action, "_", 2)

	button := 0
	action := None

	switch actionParts[0] {
	case "on":
		button = 0
	case "up":
		button = 1
	case "down":
		button = 2
	case "off":
		button = 3
	}

	switch actionParts[1] {
	case "press":
		action = Single
	case "hold":
		action = Long
	}

	return button, action
}

func mappingSimple(update DeviceUpdate) (int, ButtonAction) {
	switch update.Action {
	case "single":
		return 0, Single
	case "double":
		return 0, Double
	case "long":
		return 0, Long
	}

	return 0, None
}

func mappingDouble(update DeviceUpdate) (int, ButtonAction) {
	switch update.Action {
	case "open":
		return 0, Single
	case "close":
		return 1, Single
	}

	return 0, None
}

func (m *Manager) Update(u DeviceUpdate) {
	bd, ok := m.Devices[u.IEEEAddress]
	if !ok {
		return
	}

	if bd.SupportsBattery && bd.HKBattery != nil {
		battVal := int(math.Round(u.Battery))

		if err := bd.HKBattery.BatteryLevel.SetValue(battVal); err != nil {
			m.logger.Error("Failed to set battery level", "err", err)
		}

		if battVal < 20 {
			bd.HKBattery.StatusLowBattery.SetValue(characteristic.StatusLowBatteryBatteryLevelLow)
		} else {
			bd.HKBattery.StatusLowBattery.SetValue(characteristic.StatusLowBatteryBatteryLevelNormal)
		}
	}

	if len(u.Action) == 0 {
		return
	}

	n, a := bd.MappingFunction(u)
	button := bd.Buttons[n]

	m.UpdateButton(button, bd, a)
}

func (m *Manager) UpdateButton(b *Button, bd *ButtonDevice, a ButtonAction) {
	switch a {
	case Single:
		b.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventSinglePress)
	case Double:
		b.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventDoublePress)
	case Long:
		b.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventLongPress)
	default:
		if bd.SynthesiseButtons {
			m.SynthesiseButtonInput(b, a)
		}
	}
}

const PressActionTimeout = 300 * time.Millisecond

func (m *Manager) SynthesiseButtonInput(b *Button, a ButtonAction) {
	pa := b.LastAction
	b.LastAction = a

	b.ActionTime = time.Now()

	if a == Held && pa != Held {
		b.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventLongPress)
	}

	if a == Release && pa == Press {
		if b.PressChain == 1 {
			b.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventDoublePress)
			b.LastAction = None
			b.PressChain = 0
		} else {
			b.PressChain = 1
		}
	} else if a == Release && pa == Held {
		b.LastAction = None
	}
}

func (m *Manager) SynthesiseButtonProcess(b *Button) {
	if b.LastAction == Release && time.Since(b.ActionTime) > PressActionTimeout {
		b.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventSinglePress)
		b.LastAction = None
		b.PressChain = 0
	}
}

func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, bd := range m.Devices {
				if bd.SynthesiseButtons {
					for _, button := range bd.Buttons {
						m.SynthesiseButtonProcess(button)
					}
				}
			}
		}
	}
}

func (m *Manager) UpdateHomeKit() {
	if !m.DevicesDirty {
		return
	}

	m.DevicesDirty = false

	m.logger.Info("Refreshing HomeKit Server.")

	var buttons []*ButtonDevice

	for _, v := range m.Devices {
		buttons = append(buttons, v)
	}

	m.HKManager.Restart(buttons)
}
