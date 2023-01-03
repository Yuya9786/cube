package task

import "github.com/looplab/fsm"

// State
const (
	Pending   string = "Pending"
	Scheduled        = "Scheduled"
	Running          = "Running"
	Completed        = "Completed"
	Failed           = "Failed"
)

// Action
const (
	Schedule string = "Schedule"
	Start    string = "Start"
	Stop     string = "Stop"
	Fail     string = "Fail"
	Restart  string = "Restart"
)

func NewFSM() *fsm.FSM {
	return fsm.NewFSM(
		Pending,
		fsm.Events{
			{Name: Schedule, Src: []string{Pending}, Dst: Scheduled},
			{Name: Start, Src: []string{Scheduled}, Dst: Running},
			{Name: Stop, Src: []string{Running}, Dst: Completed},
			{Name: Fail, Src: []string{Scheduled, Running}, Dst: Failed},
			{Name: Restart, Src: []string{Running, Completed, Failed}, Dst: Running},
		},
		fsm.Callbacks{},
	)
}
