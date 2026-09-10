package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocolly/colly/v2"
)

type menu struct {
	Day       string   `json:"day"`
	Date      string   `json:"date"`
	Breakfast []string `json:"breakfast"`
	Lunch     []string `json:"lunch"`
	Dinner    []string `json:"dinner"`
}

var (
	cachedMenu []menu
	fetchTime  time.Time
	cacheMutex sync.RWMutex // this was new for me, had to look up caching methods
)

const cacheDuration = time.Hour

func fetchMenu() ([]menu, error) { // used AI in some parts of analyzing the html response and getting the element names/css classes
	c := colly.NewCollector()
	var allDays []menu
	var scrapeErr error // error variable

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
			if err := json.Unmarshal([]byte(cleanJSON), &allDays); err != nil {
				scrapeErr = fmt.Errorf("unexpected json format: %v", err)
			}
		} else {
			scrapeErr = fmt.Errorf("regex match failed, unexpected html response")
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		scrapeErr = fmt.Errorf("network error in hitting ssms-pilani.in, site is either down or server has been black listed\n%v", err)
	})

	c.Visit("https://www.ssms-pilani.in/")

	// if any error ocurrs during the scrape, return it
	if scrapeErr != nil {
		return nil, scrapeErr
	}

	return allDays, nil
}

func getMenu(ctx *gin.Context) {
	// check if cache is valid (using a read lock so multiple users can read at once)
	cacheMutex.RLock()
	isCacheValid := time.Since(fetchTime) < cacheDuration && len(cachedMenu) > 0
	cacheMutex.RUnlock()

	if isCacheValid {
		ctx.IndentedJSON(http.StatusOK, cachedMenu) // serve instantly from memory
		return
	}

	// cache expired or missing, fetch new data
	newData, err := fetchMenu()
	if err != nil {
		// if SSMS site is down, try to serve stale cached data instead of breaking the app as it is likely to contain upcoming menus anywaysd
		cacheMutex.RLock()
		hasStaleCache := len(cachedMenu) > 0
		cacheMutex.RUnlock()

		if hasStaleCache {
			ctx.IndentedJSON(http.StatusOK, cachedMenu)
			return
		}

		// if SSMS is down and no cache is stored
		ctx.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// successfully fetched new data, update the cache securely (using a write Lock)
	cacheMutex.Lock()
	cachedMenu = newData
	fetchTime = time.Now()
	cacheMutex.Unlock()

	ctx.IndentedJSON(http.StatusOK, cachedMenu) // serve the scraped menu as json
}

func main() {
	router := gin.Default() // initiate the webserver with gin

	// this part ideally shouldn't be here, it is to make things easier for interacting with the react part
	// but this shouldn't really matter cause the user is interacting over an app and not a web based frontend
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Next()
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // fallback port for testing
	}
	router.GET("/api", getMenu)   // decide endpoint and its controling function
	router.Run("0.0.0.0:" + port) // controls where the server is to be run
}
