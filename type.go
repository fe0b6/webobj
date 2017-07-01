package webobj

import (
	"net/http"
	"time"
)

type InitObj struct {
	YateScript    string
	Domain        string
	CookieName    string
	CookieTime    int
	CookieSecure  bool
	FontsEnabled  bool
	Fonts         []string
	FontsWaitTime int64
	CspEnabled    bool
	Csp           string
	CheckSession  func(string) interface{}
	CheckCsrf     func(interface{}, string, string) bool
	InlineCss     bool
}

type RqObj struct {
	W             http.ResponseWriter
	R             *http.Request
	Ans           AnswerObj
	TimeStart     time.Time
	User          interface{}
	FontChan      chan string
	Cache         map[string]interface{}
	AppendFunc    func(*RqObj, map[string]interface{}) map[string]interface{}
	CheckCsrfTmp  func(interface{}, string, string) bool
	StopInlineCss bool
}

type AnswerObj struct {
	Path     []string
	Redirect string
	Cookie   string
	Data     interface{}
	Exited   bool
	Code     int
	Meta     AnswerObjMeta
}

type AnswerObjMeta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}
