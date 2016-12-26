package main

import (
	"fmt"
	"log"

	"github.com/PuerkitoBio/goquery"
	"github.com/docopt/docopt-go"
)

const website = "http://www.theindependentsf.com/"

func main() {
	usage := `Spotify Playlists Creator

Usage:
  spotify-playlists
  spotify-playlists -h | --help

Options:
  -h --help     Show this screen.`

	arguments, _ := docopt.Parse(usage, nil, true, "", false)
	fmt.Println(arguments)

	scrapeArtists()
}

func scrapeArtists() {
	doc, err := goquery.NewDocument(website)
	if err != nil {
		log.Fatal(err)
	}

	doc.Find("div .list-view-item").Each(func(index int, item *goquery.Selection) {
		link, _ := item.Find("a").Attr("href")
		detailsTag := item.Find("div .list-view-details")
		date := detailsTag.Find(".dates").Text()
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
