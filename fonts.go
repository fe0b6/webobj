package webobj

import (
	"crypto/md5"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"

	"github.com/fe0b6/config"
)

var (
	fontsLevelDb *leveldb.DB

	inlineFontsTarget *regexp.Regexp
)

func init() {
	inlineFontsTarget = regexp.MustCompile(`<meta type="fonts"/>`)
}

func fontsInit() {
	var err error
	path := config.GetStr("main", "path") + config.GetStr("web", "fonts", "cache_path")

	if config.GetBool("web", "fonts", "clean_on_start") {
		err = os.RemoveAll(path)
		if err != nil {
			log.Fatalln("[fatal]", err)
			os.Exit(2)
			return
		}
	}

	// Открываем хранилище
	fontsLevelDb, err = leveldb.OpenFile(path, &opt.Options{
		NoSync: true,
	})

	if err != nil {
		log.Fatalln("[fatal]", err)
		os.Exit(2)
		return
	}
}

// Вставляем шрифты в html
func fontsInsert(html, font string) string {
	return inlineFontsTarget.ReplaceAllString(html, font)
}

// Формируем строку со ссылками на шрифты
func fontsGetDefault() string {
	fonts := make([]string, len(initObj.Fonts))

	for i, f := range initObj.Fonts {
		fonts[i] = `<link href="` + f + `" rel="stylesheet" type="text/css" />`
	}

	return strings.Join(fonts, "")
}

// Получаем шрифты
func (ro *RqObj) GetFonts(r *http.Request) {
	// Если шрифты отключены
	if !initObj.FontsEnabled || len(initObj.Fonts) == 0 {
		return
	}

	ua := r.Header.Get("User-Agent")
	// check cache
	font, err := getFontFromCache(ua)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	// Если стиль нашли - возвращаем
	if font != "" {
		// Отправляем шрифт в канал
		ro.sendFont(font)
		return
	}

	// Канал для сбора ответов
	ansChan := make(chan string, len(initObj.Fonts))

	// Идем получать шрифты
	var wg sync.WaitGroup
	for _, url := range initObj.Fonts {
		wg.Add(1)
		go ro.getFont(url, ua, ansChan, &wg)
	}
	// Ждем пока получим все шрифты
	wg.Wait()

	// Читаем ответы
	ans := []string{}
	for v := range ansChan {
		ans = append(ans, v)
		if len(ansChan) == 0 {
			break
		}
	}

	font = strings.Join(ans, "\n")

	// минифицируем стили
	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	mf, err := m.String("text/css", font)
	if err != nil {
		log.Println("[error]", err)
	} else {
		font = mf
	}

	// Отправляем шрифт в канал
	ro.sendFont(font)

	// Сохраняем стиль в кэш
	setFontToCache(ua, font)
}

// Отправляем шрифт в канал
func (ro *RqObj) sendFont(f string) {
	// Отправляем шрифт в канал
	ro.FontChan <- "<style>" + f + "</style>"
	// Закрываем канал
	close(ro.FontChan)
}

// Получаем шрифт
func (ro *RqObj) getFont(url, ua string, ansChan chan string, wg *sync.WaitGroup) {
	defer wg.Done()

	// Формируем запрос
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	// Добавляем заголовок
	req.Header.Set("User-Agent", ua)

	// Отправлякм запрос
	client := &http.Client{}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Println("[error]", err)
		return
	}

	if resp.StatusCode != 200 {
		log.Println("[error]", resp.StatusCode)
		return
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	// Отправляем ответ
	ansChan <- string(body)

	return
}

// Получаем кэш шрифтов
func getFontFromCache(ua string) (fonts string, err error) {

	// Ищем токен стиля для агента
	token, err := fontsLevelDb.Get([]byte("ua_token_"+ua), nil)
	if err != nil && err.Error() != "leveldb: not found" {
		log.Println("[error]", err)
		return
	}
	if len(token) == 0 {
		err = nil
		return
	}

	// Получаем сам стиль
	style, err := fontsLevelDb.Get(token, nil)
	if err != nil && err.Error() != "leveldb: not found" {
		log.Println("[error]", err)
		return
	}
	if len(style) == 0 {
		err = nil
		return
	}

	fonts = string(style)
	return
}

// Сохраняем стиль в кэш
func setFontToCache(ua, font string) {
	h := md5.New()
	_, err := h.Write([]byte(font))
	if err != nil {
		log.Println("[error]", err)
		return
	}

	fb := h.Sum(nil)

	// Пишем инфу в базу
	err = fontsLevelDb.Put([]byte("ua_token_"+ua), fb, nil)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	err = fontsLevelDb.Put(fb, []byte(font), nil)
	if err != nil {
		log.Println("[error]", err)
		return
	}
}
