package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oguzhankaracabay/hostshift/internal/safety"
)

type HostRole string

const (
	HostRoleSource HostRole = "source"
	HostRoleTarget HostRole = "target"
	HostRoleLocal  HostRole = "local"
)

type Impact string

const (
	ImpactReadOnly Impact = "read-only"
	ImpactWrite    Impact = "write"
	ImpactService  Impact = "service"
	ImpactNetwork  Impact = "network"
)

type Phase string

const (
	PhaseDiscover Phase = "discover"
	PhasePlan     Phase = "plan"
	PhasePrepare  Phase = "prepare"
	PhaseSync     Phase = "sync"
	PhaseVerify   Phase = "verify"
	PhaseCutover  Phase = "cutover"
	PhaseRollback Phase = "rollback"
)

type Action struct {
	ID            string   `json:"id"`
	Phase         Phase    `json:"phase"`
	HostRole      HostRole `json:"hostRole"`
	Impact        Impact   `json:"impact"`
	Command       []string `json:"command"`
	Preconditions []string `json:"preconditions,omitempty"`
	Rollback      []string `json:"rollback,omitempty"`
}

type actionAlias Action

func (a Action) MarshalJSON() ([]byte, error) {
	type redactedAction struct {
		actionAlias
		Command []string `json:"command"`
	}
	return json.Marshal(redactedAction{
		actionAlias: actionAlias(a),
		Command:     safety.RedactArgs(a.Command),
	})
}

type StreamAction struct {
	ID            string   `json:"id"`
	Phase         Phase    `json:"phase"`
	SourceCommand []string `json:"sourceCommand"`
	TargetCommand []string `json:"targetCommand"`
	Preconditions []string `json:"preconditions,omitempty"`
	Rollback      []string `json:"rollback,omitempty"`
}

type streamAlias StreamAction

func (s StreamAction) MarshalJSON() ([]byte, error) {
	type redactedStream struct {
		streamAlias
		SourceCommand []string `json:"sourceCommand"`
		TargetCommand []string `json:"targetCommand"`
	}
	return json.Marshal(redactedStream{
		streamAlias:   streamAlias(s),
		SourceCommand: safety.RedactArgs(s.SourceCommand),
		TargetCommand: safety.RedactArgs(s.TargetCommand),
	})
}

func (a Action) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("action id is required")
	}
	if len(a.Command) == 0 {
		return fmt.Errorf("action %s has no command", a.ID)
	}
	if a.HostRole == HostRoleSource && a.Impact != ImpactReadOnly {
		return fmt.Errorf("source action %s must be read-only", a.ID)
	}
	return nil
}

func ValidatePlan(actions []Action) error {
	for _, action := range actions {
		if err := action.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s StreamAction) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("stream action id is required")
	}
	if len(s.SourceCommand) == 0 {
		return fmt.Errorf("stream action %s has no source command", s.ID)
	}
	if len(s.TargetCommand) == 0 {
		return fmt.Errorf("stream action %s has no target command", s.ID)
	}
	return nil
}

func ValidateStreams(streams []StreamAction) error {
	for _, stream := range streams {
		if err := stream.Validate(); err != nil {
			return err
		}
	}
	return nil
}
