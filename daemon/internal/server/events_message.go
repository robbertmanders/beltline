package server

import (
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
	"strings"
)

// EventResource is one resource the change events name. Its raw values are the
// wire names.
type EventResource string

// The EventResource values, in declaration order.
const (
	EventResourceRuns      EventResource = "runs"
	EventResourceLibrary   EventResource = "library"
	EventResourceSettings  EventResource = "settings"
	EventResourceState     EventResource = "state"
	EventResourceIntake    EventResource = "intake"
	EventResourceAssistant EventResource = "assistant"
	EventResourceAfk       EventResource = "afk"
	EventResourceModels    EventResource = "models"
)

// AllEventResources is every EventResource value, in order.
var AllEventResources = []EventResource{
	EventResourceRuns, EventResourceLibrary, EventResourceSettings, EventResourceState, EventResourceIntake,
	EventResourceAssistant, EventResourceAfk, EventResourceModels,
}

// Less is the raw strings compare in byte order.
func (r EventResource) Less(other EventResource) bool {
	return strings.Compare(string(r), string(other)) < 0
}

// EventMessageKind is which of the three application messages a value is.
type EventMessageKind string

// The EventMessageKind values (the wire `type`).
const (
	EventMessageHello        EventMessageKind = "hello"
	EventMessageChanged      EventMessageKind = "changed"
	EventMessageNotification EventMessageKind = "notification"
)

// EventMessage is one of the three application messages on /api/v1/events. The
// The event message is one struct: Kind selects the case and only
// that case's fields are used. Encoded keys: hello {apiVersion, type};
// changed {resource, revision, type}; notification {body, title, type}.
type EventMessage struct {
	Kind EventMessageKind
	// APIVersion is set for hello.
	APIVersion int
	// Resource and Revision are set for changed.
	Resource EventResource
	Revision uint64
	// Title and Body are set for notification.
	Title string
	Body  string
}

// NewHelloMessage builds the hello message from its arguments.
func NewHelloMessage(apiVersion int) EventMessage {
	return EventMessage{Kind: EventMessageHello, APIVersion: apiVersion}
}

// NewChangedMessage builds the changed message from its arguments.
func NewChangedMessage(resource EventResource, revision uint64) EventMessage {
	return EventMessage{Kind: EventMessageChanged, Resource: resource, Revision: revision}
}

// NewNotificationMessage builds the notification message from its arguments.
func NewNotificationMessage(title, body string) EventMessage {
	return EventMessage{Kind: EventMessageNotification, Title: title, Body: body}
}

// helloWire, changedWire and notificationWire are the encoded shapes, one per
// case, their keys in the order they have always been sent.
type helloWire struct {
	APIVersion int    `json:"apiVersion"`
	Type       string `json:"type"`
}

type changedWire struct {
	Resource string `json:"resource"`
	Revision uint64 `json:"revision"`
	Type     string `json:"type"`
}

type notificationWire struct {
	Body  string `json:"body"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// MarshalJSON only the keys of the case, plus `type`.
func (m EventMessage) MarshalJSON() ([]byte, error) {
	switch m.Kind {
	case EventMessageChanged:
		return jsonx.Marshal(changedWire{Resource: string(m.Resource), Revision: m.Revision, Type: string(m.Kind)})
	case EventMessageNotification:
		return jsonx.Marshal(notificationWire{Body: m.Body, Title: m.Title, Type: string(m.Kind)})
	default:
		return jsonx.Marshal(helloWire{APIVersion: m.APIVersion, Type: string(EventMessageHello)})
	}
}
