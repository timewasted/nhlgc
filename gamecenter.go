// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package nhlgc is a library that interacts with the NHL GameCenter API.
package nhlgc

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"

	"github.com/grafov/m3u8"
)

// defaultUserAgent is the default value used for the User-Agent HTTP header.
// The User-Agent influences the master playlist that is returned to us, so
// changing this here isn't necessarily straightforward.
const defaultUserAgent = "Mozilla/5.0 (iPad; CPU OS 8_1 like Mac OS X) AppleWebKit/600.1.4 (KHTML, like Gecko) Version/8.0 Mobile/12B410 Safari/600.1.4"

// API endpoints.
const (
	consoleURL        = "https://gamecenter.nhl.com/nhlgc/servlets/simpleconsole"
	loginURL          = "https://gamecenter.nhl.com/nhlgc/secure/login"
	gamesListURL      = "https://gamecenter.nhl.com/nhlgc/servlets/games"
	gameInfoURL       = "https://gamecenter.nhl.com/nhlgc/servlets/game"
	publishPointURL   = "https://gamecenter.nhl.com/nhlgc/servlets/publishpoint"
	gameHighlightsURL = "http://video.nhl.com/videocenter/servlets/playlist"
)

// Error messages.
const (
	err200Expected        = "Expected a status code of 200"
	errM3U8Decode         = "m3u8 decode: "
	errM3U8ExpectedMaster = "Expected a master m3u8 playlist"
	errM3U8ExpectedMedia  = "Expected a media m3u8 playlist"
	errJSONUnmarshal      = "JSON unmarshal: "
	errXMLUnmarshal       = "XML unmarshal: "
)

// Valid stream types for the publishPoint API endpoint.
const (
	StreamTypeArchive   = "archive"
	StreamTypeCondensed = "condensed"
	StreamTypeDVR       = "dvr"
	StreamTypeLive      = "live"
)

// Stream sources.
const (
	StreamSourceHome   = "2"
	StreamSourceAway   = "4"
	StreamSourceFrench = "8"
)

// Season type identifiers.
const (
	SeasonTypePre  = "01"
	SeasonTypeReg  = "02"
	SeasonTypePost = "03"
)

type NHLGameCenter struct {
	httpClient *http.Client
	plid       string
}

type GamesList struct {
	Games []GameDetails `xml:"games>game"`
}

type GameInfo struct {
	Game GameDetails `xml:"game"`
}

type GameDetails struct {
	GID           string         `xml:"gid"`
	Season        string         `xml:"season"`
	Type          string         `xml:"type"`
	ID            string         `xml:"id"`
	Date          GameTimeGMT    `xml:"date"`
	GameStartTime GameTimeGMT    `xml:"gameTimeGMT"`
	GameEndTime   GameTimeGMT    `xml:"gameEndTimeGMT"`
	HomeTeam      string         `xml:"homeTeam"`
	AwayTeam      string         `xml:"awayTeam"`
	HomeGoals     OptionalUint64 `xml:"homeGoals"`
	AwayGoals     OptionalUint64 `xml:"awayGoals"`
	Blocked       bool           `xml:"blocked"`
	GameState     string         `xml:"gameState"`
	Result        string         `xml:"result"`
	IsLive        bool           `xml:"isLive"`
	PublishPoint  string         `xml:"program>publishPoint"`
}

type GameHighlights map[string]GameHighlight

type GameHighlight struct {
	ID           string `json:"id"`
	PublishPoint string `json:"publishPoint"`
}

type GamePublishPoint struct {
	Path string `xml:"path"`
}

type StreamPlaylist struct {
	RawFile   string
	M3U8      m3u8.Playlist
	URL       *url.URL
	Bandwidth uint32
}
type ByHighestBandwidth []StreamPlaylist

func (a ByHighestBandwidth) Len() int           { return len(a) }
func (a ByHighestBandwidth) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByHighestBandwidth) Less(i, j int) bool { return a[i].Bandwidth > a[j].Bandwidth }

type DecryptionParameters struct {
	Method   string
	Sequence uint64
	Key, IV  []byte
}

// New returns an instance of NHLGameCenter.
func New() *NHLGameCenter {
	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		panic("Failed to create cookie jar: " + err.Error())
	}

	plid := make([]byte, 16)
	n, err := rand.Read(plid)
	if err != nil {
		panic("Failed to get random bytes for plid: " + err.Error())
	}
	if n != 16 {
		panic(fmt.Sprintf("Failed to get 16 random bytes for plid: received %d bytes", n))
	}

	gc := &NHLGameCenter{
		httpClient: &http.Client{
			Jar: cookieJar,
		},
		plid: hex.EncodeToString(plid),
	}
	return gc
}

// Login logs into NHL GameCenter using the specified credentials. The 'rogers'
// parameter should be true when using a Rogers internet login.
func (gc *NHLGameCenter) Login(username, password string, rogers bool) error {
	const fnName = "Login"

	params := url.Values{}
	params.Set("username", username)
	params.Set("password", password)
	if rogers {
		params.Set("rogers", "true")
	}

	response, err := gc.getResponse(fnName, "POST", loginURL, nil, params)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	return nil
}

// GetRecentGames retrieves a list of recent and upcoming games.
func (gc *NHLGameCenter) GetRecentGames() (GamesList, error) {
	const fnName = "GetRecentGames"
	return gc.getGames(fnName, false)
}

// GetTodaysGames retrieves a list of games that take place today.
func (gc *NHLGameCenter) GetTodaysGames() (GamesList, error) {
	const fnName = "GetTodaysGames"
	return gc.getGames(fnName, true)
}

func (gc *NHLGameCenter) getGames(caller string, todayOnly bool) (games GamesList, err error) {
	params := url.Values{}
	params.Set("format", "xml")
	params.Set("isFlex", "true")
	if todayOnly {
		params.Set("app", "true")
	}

	response, err := gc.getResponseBody(caller, "POST", gamesListURL, nil, params)
	if err != nil {
		return
	}

	if err = xml.Unmarshal(response, &games); err != nil {
		err = newLogicError(caller, errXMLUnmarshal+err.Error())
		return
	}
	for i, game := range games.Games {
		if len(game.ID) > 0 && len(game.ID) < 4 {
			games.Games[i].ID = strings.Repeat("0", 4-len(game.ID)) + game.ID
		}
		if game.PublishPoint != "" {
			games.Games[i].PublishPoint = strings.Replace(games.Games[i].PublishPoint, "adaptive://", "http://", 1)
			games.Games[i].PublishPoint = strings.Replace(games.Games[i].PublishPoint, "_pc.mp4", ".mp4.m3u8", 1)
		}
	}

	return
}

// GetGameDetails retrieves details about the specified game.
func (gc *NHLGameCenter) GetGameDetails(season, gameID string) (game GameDetails, err error) {
	const fnName = "GetGameDetails"

	if len(gameID) > 0 && len(gameID) < 4 {
		gameID = strings.Repeat("0", 4-len(gameID)) + gameID
	}

	params := url.Values{}
	params.Set("app", "true")
	params.Set("isFlex", "true")
	params.Set("season", season)
	params.Set("gid", gameID)

	response, err := gc.getResponseBody(fnName, "POST", gameInfoURL, nil, params)
	if err != nil {
		return
	}

	var gameInfo GameInfo
	if err = xml.Unmarshal(response, &gameInfo); err != nil {
		err = newLogicError(fnName, errXMLUnmarshal+err.Error())
		return
	}
	game = gameInfo.Game

	return
}

// GetGameHighlights retrieves URLs for highlight videos for the specified
// game. The return value is a map, where the key is a label for the source of
// the highlight. For example:
//
// highlights := GameHighlights{
//	"home": GameHighlight{
//		PublishPoint: "http://example.com/home-highlights",
//	},
//	"away": GameHighlight{
//		PublishPoint: "http://example.com/away-highlights",
//	},
//	"french": GameHighlight{
//		PublishPoint: "http://example.com/highlights-in-french",
//	},
// }
func (gc *NHLGameCenter) GetGameHighlights(season, gameID string) (highlights GameHighlights, err error) {
	const fnName = "GetGameHighlights"
	highlights = map[string]GameHighlight{}

	if len(gameID) > 0 && len(gameID) < 4 {
		gameID = strings.Repeat("0", 4-len(gameID)) + gameID
	}

	baseID := season + SeasonTypeReg + gameID
	homeSuffix, awaySuffix, frenchSuffix := "-X-h", "-X-a", "-X-fr"

	params := url.Values{}
	params.Set("format", "json")
	params.Set("ids", baseID+homeSuffix+","+baseID+awaySuffix+","+baseID+frenchSuffix)

	// FIXME: This request doesn't require authentication, and since it's
	// going out over plain HTTP, we shouldn't send cookies.
	response, err := gc.getResponseBody(fnName, "GET", gameHighlightsURL, nil, params)
	if err != nil {
		return
	}
	response = bytes.TrimSpace(response)
	if len(response) == 0 {
		return
	}

	var arrHighlights []GameHighlight
	if err = json.Unmarshal(response, &arrHighlights); err != nil {
		err = newLogicError(fnName, errJSONUnmarshal+err.Error())
		return
	}
	for _, hl := range arrHighlights {
		switch hl.ID {
		case baseID + homeSuffix:
			highlights["home"] = hl
		case baseID + awaySuffix:
			highlights["away"] = hl
		case baseID + frenchSuffix:
			highlights["french"] = hl
		}
	}

	return
}

// GetGamePlaylists retrieves and parses the master playlist for the specified
// game. This playlist will generally contain multiple media playlists of
// varying stream quality.
func (gc *NHLGameCenter) GetGamePlaylists(season, gameID, streamType, streamSource string) (playlists []StreamPlaylist, err error) {
	const fnName = "GetGamePlaylists"

	if len(gameID) > 0 && len(gameID) < 4 {
		gameID = strings.Repeat("0", 4-len(gameID)) + gameID
	}

	// Get a link to the playlist.
	params := url.Values{}
	params.Set("type", "game")
	params.Set("gs", streamType)
	params.Set("ft", streamSource)
	params.Set("id", season+SeasonTypeReg+gameID)
	params.Set("plid", gc.plid)

	response, err := gc.getResponseBody(fnName, "POST", publishPointURL, nil, params)
	if err != nil {
		return
	}

	// Parse the XML that we received to get the playlist URL.
	response = bytes.Replace(response, []byte("_ipad"), []byte(""), -1)
	var pubPoint GamePublishPoint
	if err = xml.Unmarshal(response, &pubPoint); err != nil {
		err = newLogicError(fnName, errXMLUnmarshal+err.Error())
		return
	}

	return gc.GetPlaylistsFromURL(pubPoint.Path)
}

// GetPlaylistsFromURL retrieves and parses a M3U8 object from the specified
// URL. The return value is a list of master or media playlists.
func (gc *NHLGameCenter) GetPlaylistsFromURL(reqUrl string) (playlists []StreamPlaylist, err error) {
	const fnName = "GetPlaylistsFromURL"

	// Parse the URL.
	parsedURL, err := url.Parse(reqUrl)
	if err != nil {
		err = newLogicError(fnName, err.Error())
		return
	}

	// Get and parse the M3U8 object.
	response, err := gc.getResponseBody(fnName, "GET", reqUrl, nil, nil)
	if err != nil {
		return
	}
	m3u8obj, m3u8type, err := m3u8.DecodeFrom(bytes.NewBuffer(response), true)
	if err != nil {
		err = newLogicError(fnName, errM3U8Decode+err.Error())
		return
	}

	switch m3u8type {
	case m3u8.MASTER:
		playlist := m3u8obj.(*m3u8.MasterPlaylist)
		for _, variant := range playlist.Variants {
			playlists = append(playlists, StreamPlaylist{
				RawFile: string(response),
				M3U8:    m3u8obj,
				URL: &url.URL{
					Scheme:   parsedURL.Scheme,
					Host:     parsedURL.Host,
					Path:     parsedURL.Path[:strings.LastIndex(parsedURL.Path, "/")+1] + variant.URI,
					RawQuery: parsedURL.RawQuery,
				},
				Bandwidth: variant.VariantParams.Bandwidth,
			})
		}
		sort.Sort(ByHighestBandwidth(playlists))
	case m3u8.MEDIA:
		playlist := m3u8obj.(*m3u8.MediaPlaylist)
		if playlist.Key != nil {
			if playlist.Key.URI[0] == '"' && playlist.Key.URI[len(playlist.Key.URI)-1] == '"' {
				playlist.Key.URI = playlist.Key.URI[1 : len(playlist.Key.URI)-1]
			}
		}
		for i, segment := range playlist.Segments {
			if segment == nil {
				continue
			}
			if segment.Key != nil {
				if segment.Key.URI[0] == '"' && segment.Key.URI[len(segment.Key.URI)-1] == '"' {
					playlist.Segments[i].Key.URI = segment.Key.URI[1 : len(segment.Key.URI)-1]
				}
			}
			playlists = append(playlists, StreamPlaylist{
				RawFile: string(response),
				M3U8:    m3u8obj,
				URL: &url.URL{
					Scheme:   parsedURL.Scheme,
					Host:     parsedURL.Host,
					Path:     parsedURL.Path[:strings.LastIndex(parsedURL.Path, "/")+1] + segment.URI,
					RawQuery: parsedURL.RawQuery,
				},
			})
		}
	default:
		err = newLogicError(fnName, fmt.Sprintf("Unsupported m3u8 list type '%d'.", m3u8type))
	}

	return
}

// GetMediaPlaylist retrieves and parses the media playlist referenced by the
// specified master playlist.
func (gc *NHLGameCenter) GetMediaPlaylist(master StreamPlaylist) (media StreamPlaylist, err error) {
	const fnName = "GetMediaPlaylist"

	// Get and parse the M3U8 object.
	response, err := gc.getResponseBody(fnName, "GET", master.URL.String(), nil, nil)
	if err != nil {
		return
	}
	m3u8obj, m3u8type, err := m3u8.DecodeFrom(bytes.NewBuffer(response), true)
	if err != nil {
		err = newLogicError(fnName, errM3U8Decode+err.Error())
		return
	}
	if m3u8type != m3u8.MEDIA {
		err = newLogicError(fnName, errM3U8ExpectedMedia)
		return
	}

	playlist := m3u8obj.(*m3u8.MediaPlaylist)
	if playlist.Key != nil {
		if playlist.Key.URI[0] == '"' && playlist.Key.URI[len(playlist.Key.URI)-1] == '"' {
			playlist.Key.URI = playlist.Key.URI[1 : len(playlist.Key.URI)-1]
		}
	}
	for i, segment := range playlist.Segments {
		if segment == nil {
			continue
		}
		if segment.Key != nil {
			if segment.Key.URI[0] == '"' && segment.Key.URI[len(segment.Key.URI)-1] == '"' {
				playlist.Segments[i].Key.URI = segment.Key.URI[1 : len(segment.Key.URI)-1]
			}
		}
	}
	media = StreamPlaylist{
		RawFile:   string(response),
		M3U8:      playlist,
		URL:       master.URL,
		Bandwidth: master.Bandwidth,
	}

	return
}

// GetStreamDecryptionParameters reads the specified media playlist and returns
// the parameters required to decrypt each video segment.
func (gc *NHLGameCenter) GetStreamDecryptionParameters(media StreamPlaylist) (params []DecryptionParameters, err error) {
	const fnName = "GetDecryptionParameters"

	playlist, ok := media.M3U8.(*m3u8.MediaPlaylist)
	if !ok {
		err = newLogicError(fnName, errM3U8ExpectedMedia)
		return
	}
	if playlist.Key == nil {
		// The stream isn't encrypted, so do nothing.
		return
	}

	param := DecryptionParameters{}
	for i, segment := range playlist.Segments {
		if segment == nil {
			continue
		}
		param.Sequence = playlist.SeqNo + uint64(i)

		if segment.Key == nil || len(segment.Key.IV) == 0 {
			param.IV = []byte{
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				byte(param.Sequence >> 56 & 0xff),
				byte(param.Sequence >> 48 & 0xff),
				byte(param.Sequence >> 40 & 0xff),
				byte(param.Sequence >> 32 & 0xff),
				byte(param.Sequence >> 24 & 0xff),
				byte(param.Sequence >> 16 & 0xff),
				byte(param.Sequence >> 8 & 0xff),
				byte(param.Sequence & 0xff),
			}
		}

		if segment.Key == nil {
			// Reuse the previous segment's key.
		} else {
			param.Method = segment.Key.Method
			if len(segment.Key.IV) > 0 {
				param.IV = []byte(segment.Key.IV)
			}

			// FIXME: Launch a goroutine for each key retrieval?
			var response []byte
			response, err = gc.getResponseBody(fnName, "GET", segment.Key.URI, nil, nil)
			if err != nil {
				return
			}
			param.Key = response
		}

		params = append(params, param)
	}

	return
}

func (gc *NHLGameCenter) getResponse(caller, method, url string, headers http.Header, params url.Values) (resp *http.Response, err error) {
	var req *http.Request
	if method == "GET" && len(params) > 0 {
		req, err = http.NewRequest(method, url+"?"+params.Encode(), nil)
	} else {
		req, err = http.NewRequest(method, url, bytes.NewBufferString(params.Encode()))
	}
	if err != nil {
		err = newNetworkError(caller, err.Error(), 0, url)
		return
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	if method == "POST" && len(params) != 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		for i, h := range v {
			if i == 0 {
				req.Header.Set(k, h)
			} else {
				req.Header.Add(k, h)
			}
		}
	}

	resp, err = gc.httpClient.Do(req)
	if err != nil {
		err = newNetworkError(caller, err.Error(), 0, url)
		return
	}
	if resp.StatusCode != 200 {
		err = newNetworkError(caller, err200Expected, resp.StatusCode, url)
		return
	}

	return
}

func (gc *NHLGameCenter) getResponseBody(caller, method, url string, headers http.Header, params url.Values) (body []byte, err error) {
	resp, err := gc.getResponse(caller, method, url, headers, params)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}
