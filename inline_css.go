package webobj

import (
	"log"

	"github.com/fe0b6/csscut"
)

// Функция вставки css в head html
func setInlineCss(html string) (nhtml string, err error) {
	nhtml, err = csscut.CutAndInject(html)
	if err != nil {
		log.Println("[error]", err)
		return
	}
	return
}
