package webobj

import (
	"net/http"
)

// Отправляем ответ
func (ro *RqObj) SendCode() {
	ro.W.WriteHeader(ro.Ans.Code)
	// Если не 200 то добавляем статус ответа
	if ro.Ans.Code != 200 {
		ro.W.Write([]byte(http.StatusText(ro.Ans.Code)))
	}
}
