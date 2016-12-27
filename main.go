package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/docopt/docopt-go"
	_ "github.com/joho/godotenv/autoload"
	"github.com/toqueteos/webbrowser"
	"github.com/zmb3/spotify"
)

const website = "http://www.theindependentsf.com/"

// redirectURI is the OAuth redirect URI for the application.
// You must register an application at Spotify's developer portal
// and enter this value.
const redirectURI = "http://localhost:8910/spotify_auth/"

var (
	spotifyAuth = spotify.NewAuthenticator(redirectURI,
		spotify.ScopePlaylistModifyPublic+" "+spotify.ScopeUserReadPrivate)
	spotifyClient  = make(chan *spotify.Client)
	spotifySession = ""
)

func main() {
	usage := `Spotify Playlists Creator

Usage:
  spotify-playlists
  spotify-playlists -h | --help

Options:
  -h --help     Show this screen.`

	arguments, _ := docopt.Parse(usage, nil, true, "", false)
	fmt.Println(arguments)

	// scrapeArtists()
	// findTopSongs()
	spotifyAuthRequest()
}

func scrapeArtists() {
	doc, err := goquery.NewDocument(website)
	if err != nil {
		panic(err)
	}

	doc.Find("div .list-view-item").Each(func(index int, item *goquery.Selection) {
		link, _ := item.Find("a").Attr("href")
		detailsTag := item.Find("div .list-view-details")

		// Get date and extract
		dateStr := detailsTag.Find(".dates").Text()
		loc, _ := time.LoadLocation("America/Los_Angeles")
		const shortForm = "Mon 1.2.2006"

		// Add year to date string (not totally correct)
		fullDateStr := dateStr + "." + time.Now().Format("2006")
		date, _ := time.ParseInLocation(shortForm, fullDateStr, loc)

		headliners := detailsTag.Find(".headliners")
		supports := detailsTag.Find(".supports")
		fmt.Printf("Artist  #%2d: %s%s\n", index, website[:len(website)-1], link)
		fmt.Printf("      Dates: %s\n", date)
		headliners.Each(func(index int, headliner *goquery.Selection) {
			fmt.Printf("  Headliner: %s\n", headliner.Text())
		})
		supports.Each(func(index int, support *goquery.Selection) {
			fmt.Printf("    Support: %s\n", support.Text())
		})
	})
}

func findTopSongs() {
	results, err := spotify.Search("Sondre Lerche", spotify.SearchTypeArtist)
	if err != nil {
		panic(err)
	}

	// Grab most popular artist
	maxPopularity := -1
	var mostPopularArtistID spotify.ID
	for _, artist := range results.Artists.Artists {
		fmt.Println(artist.SimpleArtist.Name)
		fmt.Println("    Popularity:", artist.Popularity)
		fmt.Println("    ID:", artist.SimpleArtist.ID)

		if artist.Popularity > maxPopularity {
			maxPopularity = artist.Popularity
			mostPopularArtistID = artist.SimpleArtist.ID
		}
	}

	// Grab top songs from most popular
	topTracks, _ := spotify.GetArtistsTopTracks(mostPopularArtistID, "US")
	for _, track := range topTracks {
		fmt.Println("Track", track.SimpleTrack.Name, track.Popularity, track.ID)
	}

}

func spotifyAuthRequest() {
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

	// Create test playlist
	playlistName := "test"
	playlist, err := client.CreatePlaylistForUser(user.ID, playlistName, true)
	if err != nil {
		panic(err)
	}
	fmt.Println("Playlist", playlist.Name)

	// Add a track
	trackID := spotify.ID("37eN6ZWb7AKkObti06ShAs")
	client.AddTracksToPlaylist(user.ID, playlist.ID, trackID)
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
