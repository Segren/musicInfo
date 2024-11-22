package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(app.notFoundResponse)

	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/healthcheck", app.healthcheckHandler)

	//получение списка песен с фильтрацией и пагинацией
	router.HandlerFunc(http.MethodGet, "/songs", app.listSongsHandler)

	//получение текста песни с пагинацией по куплетам
	router.HandlerFunc(http.MethodGet, "/songs/:id/lyrics", app.getSongLyricsHandler)
	//удаление песни
	router.HandlerFunc(http.MethodDelete, "/songs/:id", app.deleteSongHandler)
	//изменение данных песни
	router.HandlerFunc(http.MethodPut, "/songs/:id", app.updateSongHandler)
	//добавление новой песни
	router.HandlerFunc(http.MethodPost, "/songs", app.createSongHandler)

	router.HandlerFunc(http.MethodGet, "/swagger/*any", httpSwagger.WrapHandler)

	standard := alice.New(
		app.recoverPanic, //обработчик для восстановления после паники
		app.rateLimit,    //обработчик для ограничения частоты запросов
	)

	return standard.Then(router)
}
