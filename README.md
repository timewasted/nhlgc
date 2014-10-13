nhlgc
=====

nhlgc is a Go library for interacting with the NHL GameCenter API.  It is currently focused on consuming video streams.

Usage is simple:

```
import(
	"github.com/timewasted/nhlgc"
	// Your other imports here
	// ...
)

gameCenter := nhlgc.New()
if err := gameCenter.Login(config.Username, config.Password); err != nil {
	panic(err)
}

games, err := gameCenter.GetTodaysGames()
if err != nil {
	panic(err)
}

playlists, err := gameCenter.GetVideoPlaylists(games.Games[0].Season, games.Games[0].ID, nhlgc.HomeTeamPlaylist)
if err != nil {
	panic(err)
}

playlist, err := gameCenter.GetStreamPlaylist(playlists[0])
if err != nil {
	panic(err)
}
```

License:
--------
```
Copyright (c) 2014, Ryan Rogers
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met: 

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer. 
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution. 

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```
