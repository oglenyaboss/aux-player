package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

type playlistRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type createPlaylistBody struct {
	Attributes struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"attributes"`
	Relationships struct {
		Tracks struct {
			Data []playlistRef `json:"data"`
		} `json:"tracks"`
	} `json:"relationships"`
}

type addPlaylistTracksBody struct {
	Data []playlistRef `json:"data"`
}

const playlistBatchSize = 25

func songRefs(ids []string) []playlistRef {
	refs := make([]playlistRef, len(ids))
	for i, id := range ids {
		refs[i] = playlistRef{ID: id, Type: "songs"}
	}
	return refs
}

func createPlaylist(ctx context.Context, api *appleAPI, name, description string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", errors.New("apple: пустой плейлист")
	}
	body := createPlaylistBody{}
	body.Attributes.Name = name
	body.Attributes.Description = description
	body.Relationships.Tracks.Data = songRefs(ids)
	data, _, err := api.post(ctx, "/me/library/playlists", body)
	if err != nil {
		return "", err
	}
	id := parseCreatedPlaylistID(data)
	if id == "" {
		return "", errors.New("apple: плейлист создан, но id не вернулся")
	}
	return id, nil
}

func addToPlaylist(ctx context.Context, api *appleAPI, playlistID string, ids []string) error {
	for start := 0; start < len(ids); start += playlistBatchSize {
		end := min(start+playlistBatchSize, len(ids))
		body := addPlaylistTracksBody{Data: songRefs(ids[start:end])}
		path := "/me/library/playlists/" + url.PathEscape(playlistID) + "/tracks"
		if _, _, err := api.post(ctx, path, body); err != nil {
			return err
		}
	}
	return nil
}

func parseCreatedPlaylistID(body []byte) string {
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil || len(r.Data) == 0 {
		return ""
	}
	return r.Data[0].ID
}
