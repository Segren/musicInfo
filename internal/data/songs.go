package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Segren/testTask/internal/validator"
)

type SongModel struct {
	DB *sql.DB
}

type Song struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"-"`
	Group       string    `json:"group"`
	Song        string    `json:"name"`
	ReleaseDate string    `json:"releaseDate"`
	Text        string    `json:"text"`
	Link        string    `json:"link"`
	Version     int32     `json:"version"`
}

type SongsResponse struct {
	Songs    []Song   `json:"songs"`
	Metadata Metadata `json:"metadata"`
}

func (m SongModel) Insert(song *Song) error {
	query := `
	    INSERT INTO songs ("group", name, releaseDate, text, link)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, version`

	args := []interface{}{song.Group, song.Song, song.ReleaseDate, song.Text, song.Link}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRowContext(ctx, query, args...).Scan(&song.ID, &song.CreatedAt, &song.Version)
}

func (m SongModel) GetAll(name string, group string, filters Filters) ([]*Song, Metadata, error) {
	query := fmt.Sprintf(`
		SELECT count(*) OVER(), id, created_at, name, "group", releaseDate, text, link, version
		FROM songs
		WHERE (to_tsvector('simple', name) @@ plainto_tsquery('simple', $1) OR $1 = '')
		AND ("group" = $2 OR $2 = '')
		ORDER BY %s %s, id ASC
		LIMIT $3 OFFSET $4`, filters.sortColumn(), filters.sortDirection())

	//контекст для прерывания запроса который длится дольше 3 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []interface{}{name, group, filters.limit(), filters.offset()}

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}

	defer rows.Close()

	totalRecords := 0
	songs := []*Song{}

	for rows.Next() {
		var song Song

		err := rows.Scan(
			&totalRecords,
			&song.ID,
			&song.CreatedAt,
			&song.Song,
			&song.Group,
			&song.ReleaseDate,
			&song.Text,
			&song.Link,
			&song.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		songs = append(songs, &song)
	}

	//проверка ошибок итерации
	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	//формирование метаданных
	metadata := calcualteMetadata(totalRecords, filters.Page, filters.PageSize)

	return songs, metadata, nil
}

func ValidateSong(v *validator.Validator, song *Song) {
	v.Check(song.Song != "", "song", "must be provided")
	v.Check(len(song.Song) <= 500, "song", "must not be more than 500 bytes long")

	v.Check(song.Group != "", "group", "must be provided")
	v.Check(len(song.Group) <= 5000, "group", "must not be more than 5000 bytes long")
}

func (m SongModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
		DELETE FROM songs
		WHERE id = $1`

	//контекст для прерывания запроса который длится дольше 3 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func (m SongModel) Get(id int64) (*Song, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT *
		FROM songs
		WHERE id = $1`

	var song Song

	//контекст для прерывания запроса который длится дольше 3 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&song.ID,
		&song.CreatedAt,
		&song.Group,
		&song.Song,
		&song.ReleaseDate,
		&song.Text,
		&song.Link,
		&song.Version,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &song, nil
}

func (m SongModel) Update(song *Song) error {
	query := `
		UPDATE songs
		SET "group" = $1, name = $2, releaseDate = $3, text=$4,version=version+1
		WHERE id = $5 AND version = $6
		RETURNING version`

	args := []interface{}{
		song.Group,
		song.Song,
		song.ReleaseDate,
		song.Text,
		song.ID,
		song.Version,
	}

	//контекст для прерывания запроса который длится дольше 3 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&song.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (m SongModel) GetLyricsByID(song *Song, id int64, page int, pageSize int) ([]string, error) {
	query := `SELECT text FROM songs WHERE id = $1`

	//контекст для прерывания запроса который длится дольше 3 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, id).Scan(&song.Text)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	// Разбиваем текст на куплеты.
	verses := strings.Split(song.Text, "\n\n")

	// Рассчитываем индексы для пагинации.
	start := (page - 1) * pageSize
	end := start + pageSize

	// Проверяем границы.
	if start >= len(verses) {
		return nil, nil // Если страница выходит за пределы.
	}
	if end > len(verses) {
		end = len(verses)
	}

	// Возвращаем куплеты для указанной страницы.
	return verses[start:end], nil
}
