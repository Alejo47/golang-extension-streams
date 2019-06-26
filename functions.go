package streamers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/go-redis/redis"
)

func GetStreamersHandlerCaching(clientId, templatePath string, redis *redis.Client) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		templ, err := template.ParseFiles(templatePath)
		if err != nil && !strings.HasSuffix(r.RequestURI, ".json") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if cache, err := redis.Get(fmt.Sprintf("%s:streams", clientId)).Result(); cache != "" && err == nil {

			var streams Streamers
			json.Unmarshal([]byte(cache), &streams)

			if streams.Total != 0 {
				if strings.HasSuffix(r.RequestURI, ".json") {
					out, _ := json.Marshal(streams)
					w.Header().Add("Content-Type", "application/json")
					w.Write(out)
				} else {
					if err := templ.Execute(w, streams); err != nil {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}
			} else if streams, err := loadStreamers(clientId); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				out, _ := json.Marshal(streams)
				if strings.HasSuffix(r.RequestURI, ".json") {
					w.Header().Add("Content-Type", "application/json")
					w.Write(out)
				} else {
					if err := templ.Execute(w, streams); err != nil {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}
				redis.Set(fmt.Sprintf("%s:streams", clientId), out, time.Minute*1).Result()
			}
		} else {
			if streams, err := loadStreamers(clientId); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				out, _ := json.Marshal(streams)
				if strings.HasSuffix(r.RequestURI, ".json") {
					w.Header().Add("Content-Type", "application/json")
					w.Write(out)
				} else {
					if err := templ.Execute(w, streams); err != nil {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}
				redis.Set(fmt.Sprintf("%s:streams", clientId), out, time.Minute*1).Result()
			}
		}
	}
}

func GetStreamersHandler(clientId, templatePath string) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {
		templ, err := template.ParseFiles(templatePath)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if streams, err := loadStreamers(clientId); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			if strings.HasSuffix(r.RequestURI, ".json") {
				out, _ := json.Marshal(streams)
				w.Header().Add("Content-Type", "application/json")
				w.Write(out)
			} else {
				if err := templ.Execute(w, streams); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}
		}
	}
}

func loadStreamers(extClientId string) (Streamers, error) {
	var streamsWG sync.WaitGroup

	var cursor string
	var done = false

	var result Streamers = Streamers{
		Total:   0,
		Streams: []Stream{},
	}

	client := &http.Client{}

	for done != true {
		uri := fmt.Sprintf("https://api.twitch.tv/extensions/%s/live_activated_channels?cursor=%s", extClientId, cursor)

		req, _ := http.NewRequest("GET", uri, nil)
		req.Header.Add("Client-Id", extClientId)
		res, _ := client.Do(req)
		body, _ := ioutil.ReadAll(res.Body)
		var streams ExtStreamsOut

		json.Unmarshal(body, &streams)
		cursor = streams.Cursor

		if len(streams.Channels) > 0 {
			streamsWG.Add(1)
			go func(s ExtStreamsOut) {
				defer streamsWG.Done()

				var usernames []string
				for _, c := range s.Channels {
					usernames = append(usernames, c.Username)
				}

				streamsUri := fmt.Sprintf("https://api.twitch.tv/helix/streams?user_login=%s", strings.Join(usernames, "&user_login="))

				streamsReq, _ := http.NewRequest("GET", streamsUri, nil)
				streamsReq.Header.Add("Client-Id", extClientId)

				streamsRes, _ := client.Do(streamsReq)
				body, _ := ioutil.ReadAll(streamsRes.Body)
				out := Streamers{}
				json.Unmarshal(body, &out)
				for i, s := range out.Streams {
					out.Streams[i].ThumbnailUrl = setRes(s.ThumbnailUrl, "360", "640")
				}
				out.parseGames(extClientId)
				result.Streams = append(result.Streams, out.Streams...)
			}(streams)
		}

		if cursor == "" {
			done = true
		}
	}

	streamsWG.Wait()

	sort.Slice(result.Streams, func(i, j int) bool {
		return result.Streams[i].Viewers > result.Streams[j].Viewers
	})

	result.Total = len(result.Streams)

	return result, nil
}

func (s Streamers) parseGames(extClientId string) Streamers {

	client := &http.Client{}

	var gameIds []string
	{
		gameIdsMap := make(map[string]bool)
		for _, g := range s.Streams {
			if !gameIdsMap[g.GameId] {
				gameIdsMap[g.GameId] = true
				gameIds = append(gameIds, g.GameId)
			}
		}
	}
	for i := 0; i < len(gameIds); i += 100 {
		var streamsUri string

		if len(gameIds) <= i+100 {
			streamsUri = fmt.Sprintf("https://api.twitch.tv/helix/games?id=%s", strings.Join(gameIds[i:], "&id="))
		} else {
			streamsUri = fmt.Sprintf("https://api.twitch.tv/helix/games?id=%s", strings.Join(gameIds[i:i+100], "&id="))
		}

		streamsReq, _ := http.NewRequest("GET", streamsUri, nil)
		streamsReq.Header.Add("Client-Id", extClientId)

		streamsRes, _ := client.Do(streamsReq)
		body, _ := ioutil.ReadAll(streamsRes.Body)
		out := struct {
			Games []Game `json:"data"`
		}{}
		json.Unmarshal(body, &out)

		for i, stream := range s.Streams {
			for _, g := range out.Games {
				if stream.GameId == g.Id {
					s.Streams[i].Game = g
				}
			}
		}
	}

	return s
}

func setRes(url, height, width string) string {
	replacer := strings.NewReplacer("{width}", width, "{height}", height)
	return replacer.Replace(url)
}
