package webobj

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fe0b6/config"
	"github.com/fe0b6/csscut"
)

const (
	NODE_SCRIPT = "/usr/bin/nodejs"
	MAX_ARG_LEN = 100000
)

var (
	initObj InitObj
)

func Init(o InitObj) chan bool {
	initObj = o

	// Проверяем inline_css
	if initObj.InlineCss {
		wwwPath := config.GetStr("web", "inline_css", "www_path")
		cachePath := config.GetStr("web", "inline_css", "cache_path")
		uncssScript := config.GetStr("nodejs", "uncss")

		if uncssScript == "" || wwwPath == "" || cachePath == "" {
			initObj.InlineCss = false
		}
	}

	// Проверяем шрифты
	if initObj.FontsEnabled {
		if len(initObj.Fonts) == 0 {
			initObj.FontsEnabled = false
		}
	}

	// Канал для оповещения о выходе
	exitChan := make(chan bool)

	// Запускаем демона inline css
	if initObj.InlineCss {
		csscut.Init(csscut.InitObj{
			WwwPath:      config.GetStr("web", "inline_css", "www_path"),
			LevelDbPath:  config.GetStr("web", "inline_css", "cache_path"),
			NodeScript:   NODE_SCRIPT,
			UncssScript:  config.GetStr("main", "path") + config.GetStr("nodejs", "uncss"),
			CleanOnStart: config.GetBool("web", "inline_css", "clean_on_start"),
		})
	}

	go waitExit(exitChan)

	// Если получение шрифтов включено
	if initObj.FontsEnabled {
		fontsInit()
	}

	// Если csp включен - формируем csp
	if initObj.CspEnabled {
		initCsp()
	}

	return exitChan
}

// Ждем сигнал о выходе
func waitExit(exitChan chan bool) {
	_ = <-exitChan
	exitChan <- true
}

func initCsp() {
	cspa := []string{}
	for _, k := range []string{"default-src", "frame-src", "object-src", "media-src",
		"font-src", "img-src", "script-src", "style-src", "connect-src", "report-uri"} {

		t := config.GetStrSilent(true, "web", "csp", k)
		if t != "" {
			cspa = append(cspa, k+" "+t)
		}
	}

	initObj.Csp = strings.Join(cspa, "; ")
}

// Проверяем JS запрос или нет
func (ro *RqObj) isJs() (ok bool) {
	if ro.R.FormValue("js") == "1" {
		ok = true
	}
	return
}

// Преобразуем объект в json и шаблонизируем
func (ro *RqObj) Tmpl() (str string, err error) {

	var o map[string]interface{}
	// Собираем данные в нужный вид
	if len(ro.Ans.Path) > 0 {
		o = map[string]interface{}{ro.Ans.Path[len(ro.Ans.Path)-1]: ro.Ans.Data}
		for i := len(ro.Ans.Path) - 2; i >= 0; i-- {
			o = map[string]interface{}{ro.Ans.Path[i]: o}
		}
	} else {
		o = map[string]interface{}{"data": ro.Ans.Data}
	}

	// Если это не js - добавляем контент
	if !ro.isJs() {
		o = map[string]interface{}{"content": o}
	} else {
		o["_tmpl"] = "main"
	}

	// Дополнительные параметры
	if ro.Ans.Meta.Title != "" {
		o["meta"] = ro.Ans.Meta
	}
	o["now_year"] = time.Now().Year()

	// Добавляем другие нужные значения
	if ro.AppendFunc != nil {
		o = ro.AppendFunc(ro, o)
	}

	// Делаем json
	var js []byte
	js, err = json.Marshal(o)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	// Если это js - не шаблонизируем
	if ro.isJs() {
		str = string(js)
		return
	}

	return ro.ObjToHtml(js)
}

// Преобразуем объект в html
func (ro *RqObj) ObjToHtml(js []byte) (str string, err error) {

	//log.Println(string(js))

	if len(js) > MAX_ARG_LEN {
		var f *os.File
		if f, err = ioutil.TempFile("/tmp/", "yate_tmpl_"); err != nil {
			return
		}
		defer os.Remove(f.Name())

		if _, err = f.Write(js); err != nil {
			return
		}

		if err = f.Close(); err != nil {
			return
		}

		js, err = json.Marshal(map[string]interface{}{"__filename": f.Name()})
		if err != nil {
			return
		}
	}

	cmd := exec.Command(NODE_SCRIPT, initObj.YateScript, string(js))
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("[error]", string(out))
		log.Println("[error]", len(js))
		return
	}

	str = string(out)

	// Если надо получить шрифты
	if initObj.FontsEnabled {
		// Сначала проверяем - может шрифт уже в канале ждет
		if len(ro.FontChan) > 0 {
			str = fontsInsert(str, <-ro.FontChan)
		} else {
			timeout := time.Now().Sub(ro.TimeStart).Nanoseconds() - initObj.FontsWaitTime*1000000

			if timeout <= 0 {
				str = fontsInsert(str, fontsGetDefault())
			} else {
				var font string
				select {
				case font = <-ro.FontChan:

				case <-time.After(time.Duration(timeout)):
					font = fontsGetDefault()
				}

				str = fontsInsert(str, font)
			}
		}
	}

	return
}

// Отправляем ответ
func (ro *RqObj) SendAnswer() {
	if ro.Ans.Exited {
		return
	}

	// Если нужно вернуть код
	if ro.Ans.Code != 0 {
		ro.SendCode()
		return
	}

	// Добавляем куку
	if ro.Ans.Cookie != "" {
		cookie := http.Cookie{
			Name:     initObj.CookieName,
			Domain:   initObj.Domain,
			Path:     "/",
			Value:    ro.Ans.Cookie,
			MaxAge:   initObj.CookieTime,
			HttpOnly: true,
			Secure:   initObj.CookieSecure,
		}
		http.SetCookie(ro.W, &cookie)
	}

	// Если переадресация
	if ro.Ans.Redirect != "" {
		ro.W.Header().Add("Expires", "Thu, 01 Jan 1970 00:00:01 GMT")
		http.Redirect(ro.W, ro.R, ro.Ans.Redirect, 301)
		return
	}

	// Формируем ответ
	str, err := ro.Tmpl()
	if err != nil {
		log.Println("[error]", err)
		ro.Ans.Code = 500
		ro.SendCode()
		return
	}

	// Если надо заинлайнить css
	if initObj.InlineCss && !ro.StopInlineCss {
		nstr, err := setInlineCss(str)
		if err != nil {
			log.Println("[error]", err)
		} else if nstr != "" {
			str = nstr
		}
	}

	// Добавляем csp
	if initObj.CspEnabled {
		ro.W.Header().Add("Content-Security-Policy", initObj.Csp)
	}

	ro.W.Write([]byte(str))
}

// Проверка сессии
func (ro *RqObj) CheckSession() (ok bool) {
	// Собираем данные авторизации
	key, err := ro.R.Cookie(initObj.CookieName)
	if err != nil {
		if err.Error() != "http: named cookie not present" {
			log.Println("[error]", err)
		}
		return
	}

	// Проверяем сессию
	ro.User = initObj.CheckSession(key.Value)
	if ro.User == nil {
		return
	}

	ok = true
	return
}

// Проверка запроса
func (ro *RqObj) CheckCsrf() (ok bool) {
	if ro.R.Method != "POST" {
		log.Println("[error]", "csrf bad method", ro.R.Method)
		return
	}

	// Собираем данные авторизации
	key, err := ro.R.Cookie(initObj.CookieName)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	return initObj.CheckCsrf(ro.User, key.Value, ro.R.FormValue("csrf"))
}

// Создаем объет кэша если его еще нет
func (ro *RqObj) MakeCache() {
	if ro.Cache == nil {
		ro.Cache = make(map[string]interface{})
	}
}
