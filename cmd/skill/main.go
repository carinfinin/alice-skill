package main

import (
	"github.com/carinfinin/alice-skill/cmd/skill/flags"
	"github.com/carinfinin/alice-skill/internal/gzip"
	"github.com/carinfinin/alice-skill/internal/logger"
	"go.uber.org/zap"
	"net/http"
	"strings"
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

	// создаём экземпляр приложения, пока без внешней зависимости хранилища сообщений
	appInstance := newApp(nil)

	logger.Log.Info("Running server", zap.String("address", flags.FlagRunAddr))
	// оборачиваем хендлер webhook в middleware с логированием
	return http.ListenAndServe(flags.FlagRunAddr, logger.RequestLogger(gzipMiddleWare(appInstance.webhook)))
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
