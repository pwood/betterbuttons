package main

type DeviceList []Device

type Device struct {
	IEEEAddress        string `json:"ieee_address"`
	Interviewing       bool   `json:"interviewing"`
	InterviewCompleted bool   `json:"interview_completed"`
	Manufacturer       string `json:"manufacturer"`
	ModelID            string `json:"model_id"`
	SoftwareBuildID    string `json:"software_build_id"`
}

type DeviceUpdate struct {
	IEEEAddress string  `json:"-"`
	Action      string  `json:"action"`
	Battery     float64 `json:"battery"`
}
