package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	im "github.com/CJKinni/cjk_math"
	"github.com/PuerkitoBio/goquery"
	"github.com/bradfitz/slice"
	"github.com/docopt/docopt-go"
	_ "github.com/joho/godotenv/autoload"
	"github.com/toqueteos/webbrowser"
	"github.com/zmb3/spotify"
)

const (
	website            = "http://www.theindependentsf.com/"
	venue              = "The Independent"
	timezone           = "America/Los_Angeles"
	port               = ":8910"
	authPath           = "/spotify_auth/"
	redirectURI        = "http://localhost" + port + authPath
	spotifyPermissions = spotify.ScopePlaylistModifyPublic + " " + spotify.ScopeUserReadPrivate
)

var (
	spotifyAuth    = spotify.NewAuthenticator(redirectURI, spotifyPermissions)
	spotifyClient  = make(chan *spotify.Client)
	spotifySession = ""
)

func main() {
	usage := `Spotify Playlists Creator

Usage:
  spotify-playlists [--n_artists=<n> --n_tracks=<n>]
  spotify-playlists -h | --help

Options:
  --n_artists=<n>  Min num artists in month to create playlist [default: 10]
  --n_tracks=<n>   Num tracks per artist to gather for the playlist [default: 3]
	-h --help        Show this screen.`

	// Command line arguments
	arguments, _ := docopt.Parse(usage, nil, true, "", false)
	fmt.Println("Arguments:", arguments)
	nArtists, _ := strconv.Atoi(arguments["--n_artists"].(string))
	nTracks, _ := strconv.Atoi(arguments["--n_tracks"].(string))

	// Scrape artists and create playlist
	artistsByMonth := scrapeArtists(nArtists)
	tracksByMonth := findTopSongs(artistsByMonth, nTracks)
	spotifyAuthRequest(tracksByMonth)
}

func scrapeArtists(nArtists int) map[string][]string {
	// Scrape artists from the Independent's website
	doc, err := goquery.NewDocument(website)
	if err != nil {
		panic(err)
	}

	// Record artsts by month
	artistsByMonth := make(map[string][]string)

	// Extract artist lineup
	doc.Find("div .list-view-item").Each(func(index int, div *goquery.Selection) {
		extractArtists(artistsByMonth, div)
	})

	// Prune the months with too few artists
	for month, artists := range artistsByMonth {
		if len(artists) < nArtists {
			delete(artistsByMonth, month)
		}
	}

	// Prune duplicate artists
	for month, artists := range artistsByMonth {
		artistsByMonth[month] = uniqueStrSlice(artists)
	}

	return artistsByMonth
}

func extractArtists(artistsByMonth map[string][]string, div *goquery.Selection) {
	// Find all artist listings
	detailsTag := div.Find("div .list-view-details")

	// Get date and extract
	dateStr := detailsTag.Find(".dates").Text()
	loc, _ := time.LoadLocation(timezone)
	const shortForm = "Mon 1.2.2006"

	// Add year to date string (not included on website)
	fullDateStr := dateStr + "." + time.Now().Format("2006")
	date, _ := time.ParseInLocation(shortForm, fullDateStr, loc)

	// Since dates are only in the future correct for year change-over
	if date.Month() < time.Now().Month() {
		date = date.AddDate(1, 0, 0)
	}

	// Get headliner artist names grouped by month (and year)
	mnth := fmt.Sprintf("%s.%d", date.Month(), date.Year())
	headliners := detailsTag.Find(".headliners")
	headliners.Each(func(index int, headliner *goquery.Selection) {
		artistsByMonth[mnth] = append(artistsByMonth[mnth], headliner.Text())
	})

	// Get supporting artist names grouped by month (and year)
	supports := detailsTag.Find(".supports")
	supports.Each(func(index int, support *goquery.Selection) {
		// Split on commas if multiple supporting acts
		supports := strings.Split(support.Text(), ",")
		artistsByMonth[mnth] = append(artistsByMonth[mnth], supports...)
	})
}

func findTopSongs(artistsByMonth map[string][]string, nTracks int) map[string][]spotify.ID {
	// Find top tracks for given artists
	tracksByMonth := make(map[string][]spotify.ID)
	for mnth, artists := range artistsByMonth {
		for _, artist := range artists[:] {

			// Search for artists
			result, err := spotify.Search(artist, spotify.SearchTypeArtist)
			if err != nil {
				fmt.Println("Artist not found:", err)
				continue
			}

			// If none found, skip to next artist
			if len(result.Artists.Artists) == 0 {
				fmt.Println("Artist not found:", artist)
				continue
			}

			// Grab most popular artist
			foundArtists := result.Artists.Artists
			slice.Sort(foundArtists, func(i int, j int) bool {
				return foundArtists[i].Popularity > foundArtists[j].Popularity
			})
			mostPopularArtistID := foundArtists[0].ID

			// Grab top N most popular songs per artist
			topTracks, _ := spotify.GetArtistsTopTracks(mostPopularArtistID, "US")

			// Sort tracks by popularity
			slice.Sort(topTracks, func(i int, j int) bool {
				return topTracks[i].Popularity > topTracks[j].Popularity
			})

			// Add at most top N tracks
			for _, track := range topTracks[:im.MinInt(nTracks, len(topTracks))] {
				tracksByMonth[mnth] = append(tracksByMonth[mnth], track.ID)
			}
		}
	}

	return tracksByMonth
}

func spotifyAuthRequest(tracksByMonth map[string][]spotify.ID) {
	// first start an HTTP server to handle auth redirect
	http.HandleFunc(authPath, createClient)
	go http.ListenAndServe(port, nil)

	// Goto auth page with random session id
	spotifySession = randStr(10)
	url := spotifyAuth.AuthURL(spotifySession)
	webbrowser.Open(url)

	// wait for auth to complete
	client := <-spotifyClient

	// get current user id
	user, err := client.CurrentUser()
	if err != nil {
		panic(err)
	}
	fmt.Println("You are logged in as:", user.ID)

	// Create playlist per month
	for month, tracks := range tracksByMonth {

		// Create public playlist
		playlistName := fmt.Sprintf("%s %s", venue, month)
		playlist, err := client.CreatePlaylistForUser(user.ID, playlistName, true)
		if err != nil {
			panic(err)
		}

		// Add all tracks
		for _, trackID := range tracks {
			client.AddTracksToPlaylist(user.ID, playlist.ID, trackID)
		}

		fmt.Println("Created playlist", playlist.Name)
	}
}

func createClient(w http.ResponseWriter, r *http.Request) {
	// Redirect handler for successfully authenticated
	tok, err := spotifyAuth.Token(spotifySession, r)
	if err != nil {
		panic(err)
	}

	// use the token to get an authenticated client
	client := spotifyAuth.NewClient(tok)
	fmt.Fprintf(w, "Login Completed!")
	spotifyClient <- &client
}

func randStr(strlen int) string {
	// From https://siongui.github.io/2015/04/13/go-generate-random-string/
	rand.Seed(time.Now().UTC().UnixNano())
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, strlen)
	for i := 0; i < strlen; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

func uniqueStrSlice(input []string) []string {
	// Started from https://github.com/KyleBanks/go-kit/blob/master/unique/unique.go
	// I can't believe I have to write something like this (no generics?)
	unique := make([]string, 0, len(input))
	hash := make(map[string]bool)

	for _, val := range input {
		if _, ok := hash[val]; !ok {
			hash[val] = true
			unique = append(unique, val)
		}
	}

	return unique
}
