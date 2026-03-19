package events

import "time"

type Event struct {
	ID          string    `json:"id"`
	Calendar    string    `json:"calendar"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Timezone    string    `json:"timezone"`
	Location    string    `json:"location"`
	ETag        string    `json:"etag"`
	Source      string    `json:"source"`
}

type Todo struct {
	ID              string     `json:"id"`
	Calendar        string     `json:"calendar"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Start           *time.Time `json:"start,omitempty"`
	Due             *time.Time `json:"due,omitempty"`
	Completed       *time.Time `json:"completed,omitempty"`
	AllDay          bool       `json:"allDay"`
	Timezone        string     `json:"timezone"`
	Status          string     `json:"status"`
	PercentComplete int        `json:"percentComplete"`
	ETag            string     `json:"etag"`
	Source          string     `json:"source"`
}

type Calendar struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Href        string `json:"href"`
	Source      string `json:"source"`
}

type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type CreateRequest struct {
	Calendar    string `json:"calendar"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Start       string `json:"start"`
	End         string `json:"end"`
	AllDay      bool   `json:"allDay"`
	Timezone    string `json:"timezone"`
	Location    string `json:"location"`
	DryRun      bool   `json:"dryRun"`
}

type CreateTodoRequest struct {
	Calendar        string `json:"calendar"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Start           string `json:"start"`
	Due             string `json:"due"`
	Completed       string `json:"completed"`
	AllDay          bool   `json:"allDay"`
	Timezone        string `json:"timezone"`
	Status          string `json:"status"`
	PercentComplete *int   `json:"percentComplete"`
	DryRun          bool   `json:"dryRun"`
}

type PatchRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Start       *string `json:"start"`
	End         *string `json:"end"`
	AllDay      *bool   `json:"allDay"`
	Timezone    *string `json:"timezone"`
	Location    *string `json:"location"`
	ETag        *string `json:"etag"`
	DryRun      bool    `json:"dryRun"`
}

type PatchTodoRequest struct {
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	Start           *string `json:"start"`
	Due             *string `json:"due"`
	Completed       *string `json:"completed"`
	AllDay          *bool   `json:"allDay"`
	Timezone        *string `json:"timezone"`
	Status          *string `json:"status"`
	PercentComplete *int    `json:"percentComplete"`
	ETag            *string `json:"etag"`
	DryRun          bool    `json:"dryRun"`
}

type MoveRequest struct {
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Calendar *string `json:"calendar"`
	ETag     *string `json:"etag"`
	DryRun   bool    `json:"dryRun"`
}

type EventInput struct {
	Calendar    string
	Title       string
	Description string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Timezone    string
	Location    string
}

type TodoInput struct {
	Calendar        string
	Title           string
	Description     string
	Start           *time.Time
	Due             *time.Time
	Completed       *time.Time
	AllDay          bool
	Timezone        string
	Status          string
	PercentComplete int
}
