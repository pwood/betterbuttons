package main

import (
	"context"
	"fmt"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
	"github.com/carlmjohnson/versioninfo"
	"log/slog"
	"math/rand"
	"net/http"
	"slices"
	"strconv"
	"strings"
)
import "github.com/brutella/hap"

type HomeKit struct {
	pctx context.Context
	dir  string

	server       *hap.Server
	cancelServer func()
	logger       *slog.Logger
}

func (h *HomeKit) Init(ctx context.Context, dir string) error {
	h.pctx = ctx
	h.dir = dir

	h.server = nil
	h.cancelServer = nil

	return nil
}

func (h *HomeKit) constructServer(buttons []*ButtonDevice) *hap.Server {
	fs := hap.NewFsStore(h.dir)

	bridge := accessory.NewBridge(accessory.Info{
		Name:         "BetterButtons",
		SerialNumber: "1",
		Manufacturer: "Peter Wood",
		Model:        "BetterButtons",
		Firmware:     versioninfo.Version,
	})

	var accessories []*accessory.A

	for _, b := range buttons {
		mac := strings.TrimPrefix(b.IEEEAddress, "0x")

		id, err := strconv.ParseUint(mac[4:], 16, 48)
		if err != nil {
			panic(err)
		}

		id <<= 8
		id &= 0x00000000ffffff00

		acc := accessory.New(accessory.Info{
			Name:         fmt.Sprintf("%s %s", b.Manufacturer, b.Model),
			SerialNumber: b.IEEEAddress,
			Manufacturer: b.Manufacturer,
			Model:        b.Model,
			Firmware:     b.SoftwareBuildID,
		}, accessory.TypeProgrammableSwitch)

		acc.Id = id
		id++

		h.logger.Debug("Accessory created.", "id", id)

		if b.SupportsBattery {
			s := service.NewBatteryService()
			s.Id = id
			id++
			acc.AddS(s.S)

			h.logger.Debug("Battery service created.", "id", id)

			b.HKBattery = s
		}

		for n, bb := range b.Buttons {
			s := service.NewStatelessProgrammableSwitch()
			s.Id = id
			id++

			c := characteristic.NewServiceLabelIndex()
			c.SetValue(n + 1)
			s.AddC(c.C)

			h.logger.Debug("Button programmable switch created.", "button", n, "id", id)

			bb.HKSwitch = s

			var validVals []int

			if bb.SupportsSingle {
				validVals = append(validVals, 0)
			}

			if bb.SupportsDouble {
				validVals = append(validVals, 1)
			}

			if bb.SupportsLong {
				validVals = append(validVals, 2)
			}

			s.ProgrammableSwitchEvent.ValidVals = validVals
			acc.AddS(s.S)

			s.ProgrammableSwitchEvent.OnValueUpdate(func(new, old int, r *http.Request) {
				h.logger.Debug("Publishing button event.", "id", s.Id, "action", new)
			})
		}

		accessories = append(accessories, acc)
	}

	server, err := hap.NewServer(fs, bridge.A, accessories...)
	if err != nil {
		h.logger.Error("Failed to construct new HomeKit server.", "err", err)
		return nil
	}

	d, err := fs.Get("serverPin")
	pin := string(d)

	if err != nil {
		var invalidPins []string

		for p, _ := range hap.InvalidPins {
			invalidPins = append(invalidPins, p)
		}

	makePin:
		for {
			pin = fmt.Sprintf("%08d", rand.Intn(99999999))

			if !slices.Contains(invalidPins, pin) {
				fs.Set("serverPin", []byte(pin))
				break makePin
			}
		}
	}

	server.Pin = pin

	return server
}

func (h *HomeKit) Restart(buttons []*ButtonDevice) {
	if h.server != nil {
		h.logger.Info("Stopping existing HomeKit server.")
		h.cancelServer()
	}

	h.server = h.constructServer(buttons)

	h.logger.Info("Starting new HomeKit server.", "pin", h.server.Pin)

	ctx, cancel := context.WithCancel(h.pctx)
	h.cancelServer = cancel

	go func() {
		if err := h.server.ListenAndServe(ctx); err != nil {
			h.logger.Error("Failed to start HomeKit server.", "err", err)
		}
	}()
}
