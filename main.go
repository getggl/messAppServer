package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gocolly/colly/v2"
)

const port = "8080"

type menu struct {
	Day       string   `json:"day"`
	Date      string   `json:"date"`
	Breakfast []string `json:"breakfast"`
	Lunch     []string `json:"lunch"`
	Dinner    []string `json:"dinner"`
}

func fetchMenu() []menu { // used AI in some parts of analyzing the html response and getting the element names/css classes
	c := colly.NewCollector()
	var allDays []menu
	/*
		parse the raw response body because next.js hides the other 13 days from the DOM
		tried a diff way earlier with .OnHTML instead of .OnResponse but the DOM gets updated dynamically
		from the server side depending on what you click at so that doesn't work here

		another interesting thing is that although the ssms website only shows 7 days worth of menu, if you
		scrape it and view it this way you get access to 14 days (out of which some are new and some old)
	*/
	c.OnResponse(func(r *colly.Response) {
		htmlContent := string(r.Body)

		// use regex to grab the full 14-day array hidden in the site's script tags
		re := regexp.MustCompile(`\\"menuData\\":(\[.*?\]),\\"timingsData\\"`)
		match := re.FindStringSubmatch(htmlContent)

		if len(match) >= 2 {
			// clean the escaped quotes (turn \" into ")
			cleanJSON := strings.ReplaceAll(match[1], `\"`, `"`)

			// directly load the json array into a go array
			json.Unmarshal([]byte(cleanJSON), &allDays)
		}
	})

	c.Visit("https://www.ssms-pilani.in/")
	return allDays
}

func getMenu(ctx *gin.Context) {
	ctx.IndentedJSON(http.StatusOK, fetchMenu()) // serve the scraped menu as json
}

func main() {
	router := gin.Default() // initiate the webserver with gin

	// this part ideally shouldn't be here, it is to make things easier for interacting with the react part
	// but this shouldn't really matter cause the user is interacting over an app and not a web based frontend
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Next()
	})

	router.GET("/api", getMenu)   // decide endpoint and its controling function
	router.Run("0.0.0.0:" + port) // controls where the server is to be run
}
