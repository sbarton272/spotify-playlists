package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
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
	redirectURI        = "http://localhost:8910/spotify_auth/"
	spotifyPermissions = spotify.ScopePlaylistModifyPublic + " " + spotify.ScopeUserReadPrivate
)

var (
	spotifyAuth    = spotify.NewAuthenticator(redirectURI, spotifyPermissions)
	spotifyClient  = make(chan *spotify.Client)
	spotifySession = ""
)

func main() {
	// TODO include support, how many songs per artist, specific month/year, consts
	usage := `Spotify Playlists Creator

Usage:
  spotify-playlists [--n_artists=<n>]
  spotify-playlists -h | --help

Options:
  --n_artists=<n>  Min num artists in month to create playlist [default: 10]
	-h --help          Show this screen.`

	arguments, _ := docopt.Parse(usage, nil, true, "", false)
	fmt.Println("Arguments:", arguments)

	i, _ := strconv.Atoi(arguments["--n_artists"].(string))
	artistsByMonth := scrapeArtists(i)
	tracksByMonth := findTopSongs(artistsByMonth)
	spotifyAuthRequest(tracksByMonth)
}

func scrapeArtists(nArtists int) map[string][]string {
	doc, err := goquery.NewDocument(website)
	if err != nil {
		panic(err)
	}

	// Record artsts by month
	artistsByMonth := make(map[string][]string)

	// Extract artist lineup
	doc.Find("div .list-view-item").Each(func(index int, item *goquery.Selection) {
		detailsTag := item.Find("div .list-view-details")

		// Get date and extract
		dateStr := detailsTag.Find(".dates").Text()
		loc, _ := time.LoadLocation("America/Los_Angeles")
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
			// TODO split on commas
			artistsByMonth[mnth] = append(artistsByMonth[mnth], headliner.Text())
		})

		// Get supporting artist names grouped by month (and year)
		supports := detailsTag.Find(".supports")
		supports.Each(func(index int, support *goquery.Selection) {
			artistsByMonth[mnth] = append(artistsByMonth[mnth], support.Text())
		})
	})

	// Prune the months with too few artists
	for k, v := range artistsByMonth {
		if len(v) < nArtists {
			delete(artistsByMonth, k)
		}
	}

	// TODO prune duplicates

	return artistsByMonth
}

func findTopSongs(artistsByMonth map[string][]string) map[string][]spotify.ID {
	tracksByMonth := make(map[string][]spotify.ID)
	for mnth, artists := range artistsByMonth {
		for _, artist := range artists[:] {
			// TODO move to helper function
			fmt.Println("artist", artist)

			// Search for artists
			result, err := spotify.Search(artist, spotify.SearchTypeArtist)
			if err != nil {
				fmt.Println("Error", err)
				continue
			}

			// If none found, skip to next artist
			if len(result.Artists.Artists) == 0 {
				fmt.Println(artist)
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

			// Add top N tracks
			// TODO define N
			for _, track := range topTracks[:im.MinInt(3, len(topTracks))] {
				tracksByMonth[mnth] = append(tracksByMonth[mnth], track.ID)
			}
		}
	}

	return tracksByMonth
}

func spotifyAuthRequest(tracksByMonth map[string][]spotify.ID) {
	// first start an HTTP server
	http.HandleFunc("/spotify_auth/", createClient)
	go http.ListenAndServe(":8910", nil)

	spotifySession = randStr(10)
	url := spotifyAuth.AuthURL(spotifySession)
	webbrowser.Open(url)

	// wait for auth to complete
	client := <-spotifyClient

	// use the client to make calls that require authorization
	user, err := client.CurrentUser()
	if err != nil {
		panic(err)
	}
	fmt.Println("You are logged in as:", user.ID)

	// Create playlist per month
	for mnth, tracks := range tracksByMonth {
		// Create public playlist
		playlistName := "The Independent" + mnth
		playlist, err := client.CreatePlaylistForUser(user.ID, playlistName, true)
		if err != nil {
			panic(err)
		}
		fmt.Println("Playlist", playlist.Name)

		// Add all tracks
		for _, trackID := range tracks {
			client.AddTracksToPlaylist(user.ID, playlist.ID, trackID)
		}
	}
}

func createClient(w http.ResponseWriter, r *http.Request) {
	tok, err := spotifyAuth.Token(spotifySession, r)
	if err != nil {
		fmt.Println(tok)
		panic(err)
	}

	// use the token to get an authenticated client
	client := spotifyAuth.NewClient(tok)
	fmt.Fprintf(w, "Login Completed!")
	spotifyClient <- &client

}

func randStr(strlen int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, strlen)
	for i := 0; i < strlen; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}
