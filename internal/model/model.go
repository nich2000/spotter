package model

import "time"

type CalendarEvent struct {
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Location string    `json:"location,omitempty"`
	Calendar string    `json:"calendar,omitempty"`
	Notes    string    `json:"notes,omitempty"`
}

type Reminder struct {
	Title    string     `json:"title"`
	DueDate  *time.Time `json:"dueDate,omitempty"`
	List     string     `json:"list,omitempty"`
	Priority int        `json:"priority,omitempty"`
	Notes    string     `json:"notes,omitempty"`
}

type MailMessage struct {
	ID       int       `json:"id,omitempty"`
	Subject  string    `json:"subject"`
	Sender   string    `json:"sender"`
	Date     time.Time `json:"date"`
	Preview  string    `json:"preview,omitempty"`
	Mailbox  string    `json:"mailbox,omitempty"`
	IsUnread bool      `json:"isUnread"`
}

type Note struct {
	Title       string    `json:"title"`
	BodyPreview string    `json:"bodyPreview,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
	Folder      string    `json:"folder,omitempty"`
}

type SourceData struct {
	CalendarFrom time.Time       `json:"calendarFrom,omitempty"`
	CalendarTo   time.Time       `json:"calendarTo,omitempty"`
	Calendar     []CalendarEvent `json:"calendar,omitempty"`
	Reminders    []Reminder      `json:"reminders,omitempty"`
	Mail         []MailMessage   `json:"mail,omitempty"`
	Notes        []Note          `json:"notes,omitempty"`
}

type SourceSummary struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	GeneratedAt time.Time `json:"generatedAt"`
	Content     DailyPlan `json:"content"`
}

type AppState struct {
	CalendarFrom    time.Time       `json:"calendarFrom,omitempty"`
	CalendarTo      time.Time       `json:"calendarTo,omitempty"`
	SourceSummaries []SourceSummary `json:"sourceSummaries,omitempty"`

	GeneratedAt time.Time       `json:"generatedAt"`
	Calendar    []CalendarEvent `json:"calendar"`
	Reminders   []Reminder      `json:"reminders"`
	Mail        []MailMessage   `json:"mail"`
	Notes       []Note          `json:"notes"`
	Plan        DailyPlan       `json:"plan"`
	Sources     []SourceStatus  `json:"sources"`
}

type SourceStatus struct {
	Name      string    `json:"name"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type DailyPlan struct {
	Summary string   `json:"summary"`
	Blocks  []string `json:"blocks"`
	Risks   []string `json:"risks"`
	Focus   []string `json:"focus"`
}
