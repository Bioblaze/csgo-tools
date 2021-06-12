package demoparser

import (
	"fmt"

	"github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/events"

	log "github.com/sirupsen/logrus"
)

// Inits the players and teams.
func (s *Service) handleMatchStart(e events.MatchStart) {
	s.Match.Map = s.Match.Header.MapName
	s.SidesSwitched = false

	if s.configurationService.IsDebug() {
		const msg = "Game started on map %v"
		s.debug(fmt.Sprintf(msg, s.Match.Map))
	}

	gameState := s.parser.GameState()

	// Create teams.
	ct := gameState.TeamCounterTerrorists()
	t := gameState.TeamTerrorists()

	s.Match.Teams[GetTeamIndex(t.Team(), s.SidesSwitched)] = &Team{State: t, StartedAs: common.TeamTerrorists}
	s.Match.Teams[GetTeamIndex(ct.Team(), s.SidesSwitched)] = &Team{State: ct, StartedAs: common.TeamCounterTerrorists}

	// Create players and map them to the teams.
	for _, player := range gameState.Participants().Playing() {
		if player.IsBot {
			continue
		}

		s.AddPlayer(player)
	}
}

func (s *Service) handleGamePhaseChanged(e events.GamePhaseChanged) {
	switch e.NewGamePhase {
	case common.GamePhaseInit:
		s.SidesSwitched = false
	case common.GamePhaseTeamSideSwitch:
		s.SidesSwitched = !s.SidesSwitched
	case common.GamePhaseGameEnded:
		s.Match.Duration = s.parser.CurrentTime()
	}
}

func (s *Service) handleRoundStart(e events.RoundStart) {
	if s.RoundOngoing {
		return
	}

	s.CurrentRound++
	s.RoundOngoing = true
	s.RoundStart = s.parser.CurrentTime()
	s.Match.Rounds = append(s.Match.Rounds, &Round{})

	if s.configurationService.IsDebug() {
		const msg = "Starting round %d"
		s.debug(fmt.Sprintf(msg, s.CurrentRound))
	}
}

func (s *Service) handleMVP(e events.RoundMVPAnnouncement) {
	player, err := s.getPlayer(e.Player)
	if err != nil {
		log.Error(err)
	}

	if s.configurationService.IsDebug() {
		const msg = "MVP for round %d is %v"
		s.debug(fmt.Sprintf(msg, s.CurrentRound, player.Name))
	}

	s.Match.Rounds[s.CurrentRound-1].MVP = player
}

func (s *Service) handleRoundEnd(e events.RoundEnd) {
	if !s.RoundOngoing {
		return
	}

	s.RoundOngoing = false
	round := s.Match.Rounds[s.CurrentRound-1]

	if s.configurationService.IsDebug() {
		const msg = "Ending round %d with winner %v"
		s.debug(fmt.Sprintf(msg, s.CurrentRound, e.Message))
	}

	round.Winner = s.Match.Teams[GetTeamIndex(e.Winner, s.SidesSwitched)]
	round.WinReason = e.Reason
	round.Duration = s.parser.CurrentTime() - s.RoundStart
}

func (s *Service) handleKill(e events.Kill) {
	if s.parser.GameState().IsWarmupPeriod() || s.CurrentRound == 0 {
		return
	}

	round := s.Match.Rounds[s.CurrentRound-1]
	kill := &Kill{Time: s.parser.CurrentTime(), Weapon: e.Weapon.Type, IsHeadshot: e.IsHeadshot,
		AssistedFlash: e.AssistedFlash, AttackerBlind: e.AttackerBlind, NoScope: e.NoScope,
		ThroughSmoke: e.ThroughSmoke, ThroughWall: e.IsWallBang(), IsDuringRound: s.RoundOngoing}

	victim, err := s.getPlayer(e.Victim)
	if err == nil {
		kill.Victim = victim
	}

	// Add optional killer if player died e.g. through fall damage.
	if e.Killer != nil {
		killer, err := s.getPlayer(e.Killer)
		if err == nil {
			kill.Killer = killer
		}
	}

	// Add optional assister.
	if e.Assister != nil {
		assister, err := s.getPlayer(e.Assister)
		if err == nil {
			kill.Assister = assister
		}
	}

	round.Kills = append(round.Kills, kill)
}
