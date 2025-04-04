package main

import (
	"context"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
	"log/slog"
	"math"
	"strings"
	"time"
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

	Buttons []*Button

	MappingFunction ActionMapper
}

type ActionMapper func(DeviceUpdate) (int, ButtonAction)

type Manager struct {
	Devices   map[string]*ButtonDevice
	HKManager *HomeKit
	logger    *slog.Logger
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
	}

	return nil, nil, false
}

func makeeWeLinkSNZB01P(bd *ButtonDevice) {
	bd.SupportsBattery = true

	bd.Buttons = []*Button{
		{
			Name:           "Button",
			SupportsSingle: true,
			SupportsDouble: true,
			SupportsLong:   true,
		},
	}
}

func makePhilipsRWL02X(bd *ButtonDevice) {
	bd.SupportsBattery = true

	bd.Buttons = []*Button{
		{
			Name:           "On",
			SupportsSingle: true,
			SupportsDouble: true,
			SupportsLong:   true,
		},
		{
			Name:           "Up",
			SupportsSingle: true,
			SupportsDouble: true,
			SupportsLong:   true,
		},
		{
			Name:           "Down",
			SupportsSingle: true,
			SupportsDouble: true,
			SupportsLong:   true,
		},
		{
			Name:           "Off",
			SupportsSingle: true,
			SupportsDouble: true,
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
	action := Press

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
		action = Press
	case "hold":
		action = Held
	case "press_release", "hold_release":
		action = Release
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

func (m *Manager) Update(u DeviceUpdate) {
	bd, ok := m.Devices[u.IEEEAddress]
	if !ok {
		return
	}

	if bd.SupportsBattery && bd.HKBattery != nil {
		bd.HKBattery.BatteryLevel.SetValue(int(math.Round(u.Battery)))

		if u.Battery < 20 {
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

	m.UpdateButton(button, a)
}

func (m *Manager) UpdateButton(button *Button, a ButtonAction) {
	switch a {
	case Single:
		button.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventSinglePress)
	case Double:
		button.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventDoublePress)
	case Long:
		button.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventLongPress)
	default:
		m.SynthesiseButtonInput(button, a)
	}
}

const PressActionTimeout = 300 * time.Millisecond

func (m *Manager) SynthesiseButtonInput(button *Button, a ButtonAction) {
	pa := button.LastAction
	button.LastAction = a

	button.ActionTime = time.Now()

	if a == Held && pa != Held {
		button.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventLongPress)
	}

	if a == Release && pa == Press {
		if button.PressChain == 1 {
			button.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventDoublePress)
			button.LastAction = None
			button.PressChain = 0
		} else {
			button.PressChain = 1
		}
	} else if a == Release && pa == Held {
		button.LastAction = None
	}
}

func (m *Manager) SynthesiseButtonProcess(button *Button) {
	if button.LastAction == Release && time.Since(button.ActionTime) > PressActionTimeout {
		button.HKSwitch.ProgrammableSwitchEvent.SetValue(characteristic.ProgrammableSwitchEventSinglePress)
		button.LastAction = None
		button.PressChain = 0
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
				for _, button := range bd.Buttons {
					m.SynthesiseButtonProcess(button)
				}
			}
		}
	}
}

func (m *Manager) UpdateHomeKit() {
	m.logger.Info("Refreshing HomeKit Server.")

	var buttons []*ButtonDevice

	for _, v := range m.Devices {
		buttons = append(buttons, v)
	}

	m.HKManager.Restart(buttons)
}
