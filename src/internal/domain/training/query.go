package training

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type Filter struct {
	PlayerID   player.ID
	PlayerName string
	From       *Date
	To         *Date
	Limit      int
}

type QueryRepository interface {
	List(context.Context, Filter) ([]Record, error)
	ListByPlayers(ctx context.Context, playerIDs []player.ID, from, to *Date, limit int) ([]Record, error)
}

type PlayerReader interface {
	ListByName(ctx context.Context, playerName string) ([]player.Player, error)
	GetByIDs(ctx context.Context, ids []player.ID) ([]player.Player, error)
}

type TeamReader interface {
	GetByIDs(ctx context.Context, ids []team.ID) (map[team.ID]string, error)
}

type RecordView struct {
	ID           ID
	PlayerName   string
	TeamName     string
	TrainingDate Date
	Content      string
	Reflection   string
}

type QueryService struct {
	repository QueryRepository
	players    PlayerReader
	teams      TeamReader
}

func NewQueryService(repository QueryRepository, options ...QueryServiceOption) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("training query repository is required")
	}
	service := &QueryService{repository: repository}
	for _, option := range options {
		if err := option(service); err != nil {
			return nil, err
		}
	}
	return service, nil
}

type QueryServiceOption func(*QueryService) error

func WithPlayerReader(reader PlayerReader) QueryServiceOption {
	return func(service *QueryService) error {
		if reader == nil {
			return errors.New("training player reader is required")
		}
		service.players = reader
		return nil
	}
}

func WithTeamReader(reader TeamReader) QueryServiceOption {
	return func(service *QueryService) error {
		if reader == nil {
			return errors.New("training team reader is required")
		}
		service.teams = reader
		return nil
	}
}

func (s *QueryService) List(ctx context.Context, filter Filter) ([]Record, error) {
	if filter.PlayerID == "" {
		return nil, errors.New("player id is required")
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return nil, errors.New("training date range is reversed")
	}
	return s.repository.List(ctx, filter)
}

func (s *QueryService) ListViews(ctx context.Context, filter Filter) ([]RecordView, error) {
	if s.players == nil || s.teams == nil {
		return nil, errors.New("training player and team readers are required")
	}
	filter.PlayerName = strings.TrimSpace(filter.PlayerName)
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return nil, errors.New("training date range is reversed")
	}
	if filter.PlayerName == "" && filter.PlayerID == "" && filter.From == nil && filter.To == nil {
		return nil, errors.New("training filter requires player name, player id, or date range")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	var playerIDs []player.ID
	if filter.PlayerName != "" {
		players, err := s.players.ListByName(ctx, filter.PlayerName)
		if err != nil {
			return nil, err
		}
		if len(players) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrPlayerNotFound, filter.PlayerName)
		}
		playerIDs = make([]player.ID, 0, len(players))
		for _, value := range players {
			playerIDs = append(playerIDs, value.ID())
		}
	} else if filter.PlayerID != "" {
		playerIDs = []player.ID{filter.PlayerID}
	}

	records, err := s.repository.ListByPlayers(ctx, playerIDs, filter.From, filter.To, limit)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []RecordView{}, nil
	}

	recordPlayerIDs := make([]player.ID, 0, len(records))
	seen := make(map[player.ID]struct{}, len(records))
	for _, record := range records {
		if _, ok := seen[record.PlayerID()]; ok {
			continue
		}
		seen[record.PlayerID()] = struct{}{}
		recordPlayerIDs = append(recordPlayerIDs, record.PlayerID())
	}
	players, err := s.players.GetByIDs(ctx, recordPlayerIDs)
	if err != nil {
		return nil, err
	}
	playerByID := make(map[player.ID]player.Player, len(players))
	teamIDs := make([]team.ID, 0, len(players))
	teamSeen := make(map[team.ID]struct{}, len(players))
	for _, value := range players {
		playerByID[value.ID()] = value
		if _, ok := teamSeen[value.TeamID()]; ok {
			continue
		}
		teamSeen[value.TeamID()] = struct{}{}
		teamIDs = append(teamIDs, value.TeamID())
	}
	teamNames, err := s.teams.GetByIDs(ctx, teamIDs)
	if err != nil {
		return nil, err
	}

	views := make([]RecordView, 0, len(records))
	for _, record := range records {
		value, ok := playerByID[record.PlayerID()]
		if !ok {
			continue
		}
		teamName, ok := teamNames[value.TeamID()]
		if !ok {
			continue
		}
		views = append(views, RecordView{ID: record.ID(), PlayerName: value.Name(), TeamName: teamName, TrainingDate: record.TrainingDate(), Content: record.Content(), Reflection: record.Reflection()})
	}
	return views, nil
}
