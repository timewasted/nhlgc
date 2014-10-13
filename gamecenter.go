// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package nhlgc is a library that interacts with the NHL GameCenter API.
package nhlgc

import (
	"bytes"
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
const defaultUserAgent = "iPad"

// API endpoints.
const (
	loginURL        = "https://gamecenter.nhl.com/nhlgc/secure/login"
	gamesListURL    = "https://gamecenter.nhl.com/nhlgc/servlets/games"
	gameInfoURL     = "https://gamecenter.nhl.com/nhlgc/servlets/game"
	publishPointURL = "https://gamecenter.nhl.com/nhlgc/servlets/publishpoint"
)

// Error messages.
const (
	err200Expected        = "Expected a status code of 200"
	errM3U8Decode         = "m3u8 decode: "
	errM3U8ExpectedMaster = "Expected a master m3u8 playlist"
	errM3U8ExpectedMedia  = "Expected a media m3u8 playlist"
	errXMLUnmarshal       = "XML unmarshal: "
)

// Playlist perspectives.
const (
	HomeTeamPlaylist = "2"
	AwayTeamPlaylist = "4"
)

type NHLGameCenter struct {
	httpClient *http.Client
}

type GamesList struct {
	Games []GameDetails `xml:"games>game"`
}

type GameInfo struct {
	Game GameDetails `xml:"game"`
}

type GameDetails struct {
	GameID       string `xml:"gid"`
	Season       string `xml:"season"`
	Type         string `xml:"type"`
	ID           string `xml:"id"`
	HomeTeam     string `xml:"homeTeam"`
	AwayTeam     string `xml:"awayTeam"`
	Blocked      bool   `xml:"blocked"`
	GameState    string `xml:"gameState"`
	Result       string `xml:"result"`
	IsLive       bool   `xml:"isLive"`
	HasProgram   bool   `xml:"hasProgram"`
	PublishPoint string `xml:"program>publishPoint"`

	// FIXME: Removed due to XML unmarshalling failing on empty values.
	//	HomeGoals uint `xml:"homeGoals"`
	//	AwayGoals uint `xml:"awayGoals"`
}

type GamePublishPoint struct {
	RawPath string `xml:"path"`
	URL     *url.URL
}

type VideoPlaylist struct {
	RawFile   string
	M3U8      m3u8.Playlist
	URL       *url.URL
	Bandwidth uint32
}
type ByHighestBandwidth []VideoPlaylist

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
	gc := &NHLGameCenter{
		httpClient: &http.Client{
			Jar: cookieJar,
		},
	}
	return gc
}

// Login logs into NHL Game Center using the provided credentials.
func (gc *NHLGameCenter) Login(username, password string) error {
	const fnName = "Login"

	params := url.Values{}
	params.Set("username", username)
	params.Set("password", password)

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

// GetGameInfo retrieves info about the given game.
func (gc *NHLGameCenter) GetGameInfo(season, gameID string) (game GameInfo, err error) {
	const fnName = "GetGameInfo"

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

	if err = xml.Unmarshal(response, &game); err != nil {
		err = newLogicError(fnName, errXMLUnmarshal+err.Error())
		return
	}

	return
}

// GetVideoPlaylists retrieves and parses the master playlist for the specified
// game. This playlist will generally contain multiple media playlists of
// varying stream quality.
func (gc *NHLGameCenter) GetVideoPlaylists(season, gameID, perspective string) (playlists []VideoPlaylist, err error) {
	const fnName = "GetVideoPlaylists"

	if len(gameID) > 0 && len(gameID) < 4 {
		gameID = strings.Repeat("0", 4-len(gameID)) + gameID
	}

	// FIXME: These are magic numbers, and I'm only guessing at their meaning.
	const (
		preSeason  = "01"
		regSeason  = "02"
		postSeason = "03"
	)

	// Get a link to the playlist.
	params := url.Values{}
	params.Set("type", "game")
	params.Set("gs", "live")
	params.Set("ft", perspective)
	params.Set("id", season+regSeason+gameID)

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
	pubPoint.URL, err = url.Parse(pubPoint.RawPath)
	if err != nil {
		err = newLogicError(fnName, err.Error())
		return
	}

	// Fetch and parse the playlist file.
	response, err = gc.getResponseBody(fnName, "GET", pubPoint.RawPath, nil, nil)
	if err != nil {
		return
	}
	pl, listType, err := m3u8.DecodeFrom(bytes.NewBuffer(response), true)
	if err != nil {
		err = newLogicError(fnName, errM3U8Decode+err.Error())
		return
	}

	switch listType {
	case m3u8.MASTER:
		playlist := pl.(*m3u8.MasterPlaylist)
		for _, v := range playlist.Variants {
			playlists = append(playlists, VideoPlaylist{
				RawFile: string(response),
				M3U8:    pl,
				URL: &url.URL{
					Scheme: pubPoint.URL.Scheme,
					Host:   pubPoint.URL.Host,
					Path:   pubPoint.URL.Path[:strings.LastIndex(pubPoint.URL.Path, "/")+1] + v.URI,
				},
				Bandwidth: v.VariantParams.Bandwidth,
			})
		}
		sort.Sort(ByHighestBandwidth(playlists))
	case m3u8.MEDIA:
		// FIXME: I don't think this path will ever be hit during normal operation.
		playlist := pl.(*m3u8.MediaPlaylist)
		for _, s := range playlist.Segments {
			if s == nil {
				continue
			}
			playlists = append(playlists, VideoPlaylist{
				RawFile: string(response),
				M3U8:    pl,
				URL: &url.URL{
					Scheme: pubPoint.URL.Scheme,
					Host:   pubPoint.URL.Host,
					Path:   pubPoint.URL.Path[:strings.LastIndex(pubPoint.URL.Path, "/")+1] + s.URI,
				},
			})
		}
	default:
		err = newLogicError(fnName, fmt.Sprintf("Unsupported m3u8 list type '%d'.", listType))
	}

	return
}

// GetStreamPlaylist retrieves and parses the media playlist specified by the
// provided master playlist.
func (gc *NHLGameCenter) GetStreamPlaylist(vpl VideoPlaylist) (spl VideoPlaylist, err error) {
	const fnName = "GetStreamPlaylist"

	// Request and parse the stream playlist.
	response, err := gc.getResponseBody(fnName, "GET", vpl.URL.String(), nil, nil)
	if err != nil {
		return
	}
	pl, listType, err := m3u8.DecodeFrom(bytes.NewBuffer(response), true)
	if err != nil {
		err = newLogicError(fnName, errM3U8Decode+err.Error())
		return
	}

	if listType != m3u8.MEDIA {
		err = newLogicError(fnName, errM3U8ExpectedMedia)
		return
	}

	playlist := pl.(*m3u8.MediaPlaylist)
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
	if playlist.Key != nil {
		if playlist.Key.URI[0] == '"' && playlist.Key.URI[len(playlist.Key.URI)-1] == '"' {
			playlist.Key.URI = playlist.Key.URI[1 : len(playlist.Key.URI)-1]
		}
	}
	spl = VideoPlaylist{
		RawFile:   string(response),
		M3U8:      pl,
		URL:       vpl.URL,
		Bandwidth: vpl.Bandwidth,
	}

	return
}

// GetStreamDecryptionParameters reads the provided media playlist and returns
// the parameters required to decrypt each video segment.
func (gc *NHLGameCenter) GetStreamDecryptionParameters(vpl VideoPlaylist) ([]DecryptionParameters, error) {
	const fnName = "GetStreamDecryptionParameters"
	var params []DecryptionParameters

	playlist, ok := vpl.M3U8.(*m3u8.MediaPlaylist)
	if !ok {
		return params, newLogicError(fnName, errM3U8ExpectedMedia)
	}
	if playlist.Key == nil {
		// The stream isn't encrypted, so do nothing.
		return params, nil
	}

	param := DecryptionParameters{}
	for i, segment := range playlist.Segments {
		if segment == nil {
			continue
		}
		param.Sequence = playlist.SeqNo + uint64(i)

		if segment.Key == nil || segment.Key.IV == "" {
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
			if segment.Key.IV != "" {
				param.IV = []byte(segment.Key.IV)
			}

			// FIXME: Launch a goroutine for each key retrieval?
			response, err := gc.getResponseBody(fnName, "GET", segment.Key.URI, nil, nil)
			if err != nil {
				return params, err
			}
			param.Key = response
		}

		params = append(params, param)
	}

	return params, nil
}

func (gc *NHLGameCenter) getResponse(caller, method, url string, headers http.Header, params url.Values) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, url, bytes.NewBufferString(params.Encode()))
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
