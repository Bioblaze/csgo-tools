package main

import (
	"time"

	"github.com/Cludch/csgo-tools/internal/config"
	"github.com/Cludch/csgo-tools/internal/demoparser"
	"github.com/Cludch/csgo-tools/internal/domain/entity"
	"github.com/Cludch/csgo-tools/internal/domain/match"
	"github.com/Cludch/csgo-tools/internal/domain/player"
	"github.com/Cludch/csgo-tools/pkg/demo"
	log "github.com/sirupsen/logrus"
)

const ParserVersion = 1

var configService *config.Service
var matchService *match.Service
var playerService *player.Service

// Sets up the global variables (config, db) and the logger.
func setup() {
	configService = config.NewService()
	db := entity.NewService(configService)
	matchService = match.NewService(match.NewRepositoryMongo(db))
	playerService = player.NewService(player.NewRepositoryMongo(db))

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
}

func main() {
	setup()

	log.Info("starting demoparser")

	const numJobs = 5
	matchQueue := make(chan *match.Match, numJobs)

	// Start numJobs-times parallel workers.
	for w := 1; w <= numJobs; w++ {
		go worker(matchQueue)
	}

	// Create a loop that checks for unparsed demos.
	t := time.NewTicker(time.Hour)
	for {
		// Get non-parsed matches from the db.
		nonParsedMatches, err := matchService.GetParseableMatches(ParserVersion)

		if err != nil {
			log.Fatal(err)
		}

		// Enqueue found matches.
		for _, match := range nonParsedMatches {
			matchQueue <- match
		}

		<-t.C
	}
}

// Takes a match from the channel, parses and persists it.
func worker(matches <-chan *match.Match) {
	for m := range matches {
		filename := m.Filename
		if filename == "" {
			return
		}

		parser := demoparser.NewService(configService)
		demoFile := &demo.Demo{ID: m.ID, MatchTime: m.CreatedAt, Filename: filename}

		if err := parser.Parse(configService.GetConfig().DemosDir, demoFile); err != nil {
			log.Error(err)
			continue
		}

		result := match.CreateResult(parser.Match)
		if err := matchService.UpdateResult(m, result, ParserVersion); err != nil {
			log.Error(err)
			continue
		}

		for _, t := range m.Result.Teams {
			for _, playerResult := range t.Players {
				player, err := playerService.GetPlayer(playerResult.SteamID)
				if err != nil {
					const msg = "main: unable to query player: %s"
					log.Errorf(msg, err)
					continue
				}

				playerResult.MatchID = m.ID
				if err := playerService.AddResult(player, playerResult); err != nil {
					log.Error(err)
				}
			}
		}

		const msg = "demoparser: finished parsing %s"
		log.Infof(msg, filename)
	}
}
