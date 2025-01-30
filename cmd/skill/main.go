package main

import (
	"encoding/json"
	"fmt"
	"github.com/carinfinin/alice-skill/cmd/skill/flags"
	"github.com/carinfinin/alice-skill/internal/gzip"
	"github.com/carinfinin/alice-skill/internal/logger"
	"github.com/carinfinin/alice-skill/internal/models"
	"go.uber.org/zap"
	"net/http"
	"strings"
	"time"
)

// функция main вызывается автоматически при запуске приложения
func main() {
	// обрабатываем аргументы командной строки
	flags.ParseFlags()

	if err := run(); err != nil {
		panic(err)
	}
}

// функция run будет полезна при инициализации зависимостей сервера перед запуском
func run() error {
	err := logger.Initialize(flags.FlagLogLevel)
	if err != nil {
		return err
	}

	logger.Log.Info("Running server", zap.String("address", flags.FlagRunAddr))
	// оборачиваем хендлер webhook в middleware с логированием
	return http.ListenAndServe(flags.FlagRunAddr, logger.RequestLogger(gzipMiddleWare(webhook)))
}

// функция webhook — обработчик HTTP-запроса
func webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// разрешаем только POST-запросы
		logger.Log.Debug("got request with bad method", zap.String("method", r.Method))
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// десериализуем запрос в структуру модели
	logger.Log.Debug("decoding request")
	var req models.Request
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// проверяем, что пришёл запрос понятного типа
	if req.Request.Type != models.TypeSimpleUtterance {
		logger.Log.Debug("unsupported request type", zap.String("type", req.Request.Type))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	text := "Для вас нет новых сообщений."
	//первый запрос новой сесст=ии
	if req.Session.New {
		// обрабатываем поле Timezone запроса
		tz, err := time.LoadLocation(req.Timezone)
		if err != nil {
			logger.Log.Debug("cannot parse timezone")
			w.WriteHeader(http.StatusBadRequest)
			return

		}
		// получаем текущее время в часовом поясе пользователя
		now := time.Now().In(tz)
		hour, minute, _ := now.Clock()

		// формируем текст ответа
		text = fmt.Sprintf("Точное время %d часов, %d минут. %s", hour, minute, text)

	}
	// заполняем модель ответа
	resp := models.Response{
		Response: models.ResponsePayload{
			Text: text, // Алиса проговорит новый текст
		},
		Version: "1.0",
	}
	w.Header().Set("Content-Type", "application/json")

	// сериализуем ответ сервера
	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
	logger.Log.Debug("sending HTTP 200 response")

}

func gzipMiddleWare(h http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		// по умолчанию устанавливаем оригинальный http.ResponseWriter как тот,
		// который будем передавать следующей функции
		ow := writer

		// проверяем, что клиент умеет получать от сервера сжатые данные в формате gzip
		acceptEncoding := request.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")

		if supportsGzip {
			// оборачиваем оригинальный http.ResponseWriter новым с поддержкой сжатия
			cw := gzip.NewCompressWriter(writer)
			// меняем оригинальный http.ResponseWriter на новый
			ow = cw
			// не забываем отправить клиенту все сжатые данные после завершения middleware

			defer cw.Close()
		}

		// проверяем, что клиент отправил серверу сжатые данные в формате gzip
		contentEncoding := request.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")

		if sendsGzip {
			// оборачиваем тело запроса в io.Reader с поддержкой декомпрессии
			cr, err := gzip.NewCompressReader(request.Body)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			// меняем тело запроса на новое
			request.Body = cr
			defer cr.Close()
		}
		// передаём управление хендлеру
		h.ServeHTTP(ow, request)
	}
}
