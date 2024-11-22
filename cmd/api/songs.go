package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Segren/testTask/internal/data"
	"github.com/Segren/testTask/internal/validator"
	"net/http"
	"net/url"
)

// показываем все песни
func (app *application) listSongsHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string
		Group string
		data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()

	input.Group = app.readString(qs, "group", "")
	input.Name = app.readString(qs, "name", "")

	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "id")

	input.Filters.SortSafelist = []string{"id", "group", "name", "-id", "-group", "-name"}

	songs, metadata, err := app.models.Songs.GetAll(input.Name, input.Group, input.Filters)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"songs": songs, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// добавляем новую песню
func (app *application) createSongHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Group string `json:"group"`
		Song  string `json:"song"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Запрос к внешнему API для получения дополнительных данных.
	songDetail, err := app.fetchSongDetails(input.Group, input.Song)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	song := &data.Song{
		Group:       input.Group,
		Song:        input.Song,
		ReleaseDate: songDetail.ReleaseDate,
		Text:        songDetail.Text,
		Link:        songDetail.Link,
	}

	//инициализация валидатора
	v := validator.New()

	if data.ValidateSong(v, song); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Songs.Insert(song)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/song/%d", song.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"song": song}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) fetchSongDetails(group, song string) (*data.Song, error) {
	//тут должен быть нужный внешний адрес
	url := fmt.Sprintf("http://localhost:8081/info?group=%s&song=%s", url.QueryEscape(group), url.QueryEscape(song))

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var songDetail struct {
		ReleaseDate string `json:"releaseDate"`
		Text        string `json:"text"`
		Link        string `json:"link"`
	}

	err = json.NewDecoder(resp.Body).Decode(&songDetail)
	if err != nil {
		return nil, err
	}

	return &data.Song{
		ReleaseDate: songDetail.ReleaseDate,
		Text:        songDetail.Text,
		Link:        songDetail.Link,
	}, nil
}

func (app *application) deleteSongHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	err = app.models.Songs.Delete(id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "movie successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateSongHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	song, err := app.models.Songs.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	var input struct {
		Group *string `json:"group"`
		Song  *string `json:"song"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Group != nil {
		song.Group = *input.Group
	}
	if input.Song != nil {
		song.Song = *input.Song
	}

	v := validator.New()

	if data.ValidateSong(v, song); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Songs.Update(song)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"song": song}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getSongLyricsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	song, err := app.models.Songs.Get(id)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}

	v := validator.New()

	if data.ValidateSong(v, song); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	page := app.readInt(r.URL.Query(), "page", 1, v)
	size := app.readInt(r.URL.Query(), "size", 1, v)

	lyrics, err := app.models.Songs.GetLyricsByID(song, id, page, size)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{"lyrics": lyrics}, nil)
}
