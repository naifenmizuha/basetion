package training

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

var (
	ErrPlayerNotFound      = errors.New("player not found by name")
	ErrAmbiguousPlayerName = errors.New("player name matches multiple players")
)

type Repository interface {
	Create(context.Context, Record) error
	Get(context.Context, ID) (Record, error)
	Update(context.Context, Record) error
}

type PlayerRepository interface {
	Get(context.Context, player.ID) (player.Player, error)
	ListByNameWithTeam(ctx context.Context, playerName, teamName string) ([]PlayerWithTeam, error)
}

type Clock interface{ Now() time.Time }

type PlayerWithTeam struct {
	Player   player.Player
	TeamName string
}

type Service struct {
	repository Repository
	players    PlayerRepository
	clock      Clock
}

func NewService(repository Repository, players PlayerRepository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, errors.New("training repository is required")
	}
	if players == nil {
		return nil, errors.New("training player repository is required")
	}
	if clock == nil {
		return nil, errors.New("training clock is required")
	}
	return &Service{repository: repository, players: players, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, id ID, playerName, teamName string, date Date, content, reflection string) (Record, error) {
	resolved, err := s.resolvePlayer(ctx, playerName, teamName)
	if err != nil {
		return Record{}, err
	}
	value, err := New(id, resolved.Player.ID(), date, content, reflection, s.clock.Now())
	if err != nil {
		return Record{}, err
	}
	if err := s.repository.Create(ctx, value); err != nil {
		return Record{}, err
	}
	return value, nil
}

func (s *Service) resolvePlayer(ctx context.Context, playerName, teamName string) (PlayerWithTeam, error) {
	playerName = strings.TrimSpace(playerName)
	teamName = strings.TrimSpace(teamName)
	if playerName == "" {
		return PlayerWithTeam{}, errors.New("player name is required")
	}
	players, err := s.players.ListByNameWithTeam(ctx, playerName, teamName)
	if err != nil {
		return PlayerWithTeam{}, err
	}
	switch len(players) {
	case 0:
		if teamName != "" {
			return PlayerWithTeam{}, fmt.Errorf("%w: %s is not on team %s", ErrPlayerNotFound, playerName, teamName)
		}
		return PlayerWithTeam{}, fmt.Errorf("%w: %s", ErrPlayerNotFound, playerName)
	case 1:
		return players[0], nil
	default:
		labels := make([]string, 0, len(players))
		for _, candidate := range players {
			labels = append(labels, fmt.Sprintf("%s（%s #%d）", candidate.Player.Name(), candidate.TeamName, candidate.Player.JerseyNumber()))
		}
		return PlayerWithTeam{}, fmt.Errorf("%w: %s; specify team_name to disambiguate: %s", ErrAmbiguousPlayerName, playerName, strings.Join(labels, "、"))
	}
}

func (s *Service) Update(ctx context.Context, id ID, content, reflection string) (Record, error) {
	value, err := s.repository.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if err := value.Update(content, reflection, s.clock.Now()); err != nil {
		return Record{}, err
	}
	if err := s.repository.Update(ctx, value); err != nil {
		return Record{}, err
	}
	return value, nil
}

func (s *Service) Delete(ctx context.Context, id ID) error {
	value, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := value.Delete(s.clock.Now()); err != nil {
		return err
	}
	return s.repository.Update(ctx, value)
}
