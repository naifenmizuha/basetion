package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type matchBaseArguments struct {
	HomeTeamID  string `json:"home_team_id"`
	AwayTeamID  string `json:"away_team_id"`
	ScheduledAt string `json:"scheduled_at"`
	Location    string `json:"location"`
}
type matchCreateArguments struct {
	matchBaseArguments
	Status string `json:"status"`
}
type matchUpdateArguments struct {
	MatchID string `json:"match_id"`
	matchBaseArguments
}
type matchStatusArguments struct {
	MatchID string `json:"match_id"`
	Status  string `json:"status"`
}
type matchDeleteArguments struct {
	MatchID string `json:"match_id"`
}
type lineupEntryArguments struct {
	PlayerID     string `json:"player_id"`
	BattingOrder *int   `json:"batting_order"`
	Position     string `json:"position"`
}
type lineupArguments struct {
	MatchID       string                 `json:"match_id"`
	TeamID        string                 `json:"team_id"`
	Kind          string                 `json:"kind"`
	VariantNumber *int                   `json:"variant_number"`
	VariantName   string                 `json:"variant_name"`
	Entries       []lineupEntryArguments `json:"entries"`
}
type lineupDeleteArguments struct {
	MatchID       string `json:"match_id"`
	TeamID        string `json:"team_id"`
	Kind          string `json:"kind"`
	VariantNumber *int   `json:"variant_number"`
}

type namedPlayerArguments struct {
	TeamName     string `json:"team_name"`
	JerseyNumber *int   `json:"jersey_number"`
}
type namedSituationArguments struct {
	Outs      *int                    `json:"outs"`
	HomeScore *int                    `json:"home_score"`
	AwayScore *int                    `json:"away_score"`
	Runners   []*namedPlayerArguments `json:"runners"`
}
type namedPitchArguments struct {
	Pitcher       namedPlayerArguments `json:"pitcher"`
	Batter        namedPlayerArguments `json:"batter"`
	Result        string               `json:"result"`
	BallsBefore   *int                 `json:"balls_before"`
	StrikesBefore *int                 `json:"strikes_before"`
	BallsAfter    *int                 `json:"balls_after"`
	StrikesAfter  *int                 `json:"strikes_after"`
	PitchType     string               `json:"pitch_type"`
	Velocity      *float64             `json:"velocity"`
	Zone          *int                 `json:"zone"`
	Description   string               `json:"description"`
}
type namedRunnerArguments struct {
	Runner         namedPlayerArguments  `json:"runner"`
	Result         string                `json:"result"`
	FromBase       *int                  `json:"from_base"`
	ToBase         *int                  `json:"to_base"`
	OutRecorded    *bool                 `json:"out_recorded"`
	Scored         *bool                 `json:"scored"`
	ChargedPitcher *namedPlayerArguments `json:"charged_pitcher"`
	Earned         *bool                 `json:"earned"`
	RBIBatter      *namedPlayerArguments `json:"rbi_batter"`
	Description    string                `json:"description"`
}
type namedFieldingArguments struct {
	Fielder     namedPlayerArguments `json:"fielder"`
	Position    string               `json:"position"`
	Result      string               `json:"result"`
	Description string               `json:"description"`
}
type namedPlayArguments struct {
	Inning            *int                     `json:"inning"`
	BattingOrder      *int                     `json:"batting_order"`
	Half              string                   `json:"half"`
	Batter            namedPlayerArguments     `json:"batter"`
	StartingPitcher   namedPlayerArguments     `json:"starting_pitcher"`
	Before            namedSituationArguments  `json:"before"`
	After             namedSituationArguments  `json:"after"`
	BattingResult     string                   `json:"batting_result"`
	ResultDescription string                   `json:"result_description"`
	Pitches           []namedPitchArguments    `json:"pitches"`
	RunnerOutcomes    []namedRunnerArguments   `json:"runner_outcomes"`
	FieldingOutcomes  []namedFieldingArguments `json:"fielding_outcomes"`
}
type namedLineupEntryArguments struct {
	Player       namedPlayerArguments `json:"player"`
	BattingOrder *int                 `json:"batting_order"`
	Position     string               `json:"position"`
}
type namedLineupArguments struct {
	Name    string                      `json:"name"`
	Entries []namedLineupEntryArguments `json:"entries"`
}
type legacyGameCreateArguments struct {
	HomeTeamName string               `json:"home_team_name"`
	AwayTeamName string               `json:"away_team_name"`
	ScheduledAt  string               `json:"scheduled_at"`
	Location     string               `json:"location"`
	HomeLineup   namedLineupArguments `json:"home_lineup"`
	AwayLineup   namedLineupArguments `json:"away_lineup"`
	Plays        []namedPlayArguments `json:"plays"`
}

// gameCreateArguments is intentionally compact. Team ownership is encoded by
// the lineup container and inning half, while the game domain derives counts
// and situations from the event stream.
type compactPitchArguments struct {
	Result      string   `json:"result" jsonschema:"required,description=逐球结果,enum=ball,enum=called_strike,enum=swinging_strike,enum=foul,enum=foul_tip,enum=in_play,enum=hit_by_pitch,enum=intentional_ball,enum=pitchout,enum=other"`
	PitchType   string   `json:"pitch_type,omitempty" jsonschema:"description=球种；未知时省略，不传空字符串"`
	Velocity    *float64 `json:"velocity,omitempty" jsonschema:"description=球速；未知时省略，不传 0"`
	Zone        *int     `json:"zone,omitempty" jsonschema:"description=进垒区域；未知时省略，不传 0"`
	Description string   `json:"description,omitempty" jsonschema:"description=补充说明；未知时省略"`
}
type compactRunnerArguments struct {
	JerseyNumber               *int   `json:"jersey_number" jsonschema:"required,description=跑者背号"`
	Result                     string `json:"result" jsonschema:"required,description=跑垒结果,enum=advance,enum=score,enum=force_out,enum=tag_out,enum=caught_stealing,enum=picked_off,enum=error_advance"`
	FromBase                   *int   `json:"from_base" jsonschema:"required,description=起始垒位；打者首次上垒为 0"`
	ToBase                     *int   `json:"to_base,omitempty" jsonschema:"description=目标垒位；未知或出局时省略"`
	ChargedPitcherJerseyNumber *int   `json:"charged_pitcher_jersey_number,omitempty" jsonschema:"description=责任投手背号；未知时省略"`
	Earned                     *bool  `json:"earned,omitempty" jsonschema:"description=是否自责分；未知时省略，不传 false"`
	RBIBatterJerseyNumber      *int   `json:"rbi_batter_jersey_number,omitempty" jsonschema:"description=打点打者背号；没有时省略"`
	Description                string `json:"description,omitempty" jsonschema:"description=补充说明；未知时省略"`
}
type compactFieldingArguments struct {
	JerseyNumber *int   `json:"jersey_number" jsonschema:"required,description=守备员背号"`
	Position     string `json:"position" jsonschema:"required,description=守备位置,enum=pitcher,enum=catcher,enum=first_base,enum=second_base,enum=shortstop,enum=third_base,enum=outfielder"`
	Result       string `json:"result" jsonschema:"required,description=守备结果,enum=putout,enum=assist,enum=error,enum=double_play,enum=triple_play,enum=passed_ball,enum=catcher_interference,enum=other"`
	Description  string `json:"description,omitempty" jsonschema:"description=补充说明；未知时省略"`
}
type compactPlayArguments struct {
	Inning              *int                       `json:"inning" jsonschema:"required,description=局数，从 1 开始"`
	Half                string                     `json:"half" jsonschema:"required,description=top 为客队进攻，bottom 为主队进攻,enum=top,enum=bottom"`
	BattingOrder        *int                       `json:"batting_order" jsonschema:"required,description=本打席打者的棒次"`
	BatterJerseyNumber  *int                       `json:"batter_jersey_number" jsonschema:"required,description=本打席打者背号"`
	PitcherJerseyNumber *int                       `json:"pitcher_jersey_number" jsonschema:"required,description=本打席投手背号"`
	BattingResult       string                     `json:"batting_result" jsonschema:"required,description=打席结果,enum=single,enum=double,enum=triple,enum=home_run,enum=walk,enum=intentional_walk,enum=hit_by_pitch,enum=strikeout,enum=ground_out,enum=fly_out,enum=line_out,enum=fielders_choice,enum=reached_on_error,enum=sacrifice_bunt,enum=sacrifice_fly,enum=interference,enum=other"`
	ResultDescription   string                     `json:"result_description" jsonschema:"required,description=打席结果说明"`
	Pitches             []compactPitchArguments    `json:"pitches" jsonschema:"required,description=按实际顺序的非空逐球数组"`
	RunnerOutcomes      []compactRunnerArguments   `json:"runner_outcomes,omitempty" jsonschema:"description=跑垒结果；没有时省略"`
	FieldingOutcomes    []compactFieldingArguments `json:"fielding_outcomes,omitempty" jsonschema:"description=守备结果；没有时省略"`
}
type compactLineupEntryArguments struct {
	JerseyNumber *int   `json:"jersey_number" jsonschema:"required,description=首发球员背号"`
	BattingOrder *int   `json:"batting_order" jsonschema:"required,description=首发棒次"`
	Position     string `json:"position" jsonschema:"required,description=首发守备位置,enum=pitcher,enum=catcher,enum=first_base,enum=second_base,enum=shortstop,enum=third_base,enum=outfielder"`
}
type compactLineupArguments struct {
	Name    string                        `json:"name" jsonschema:"required,description=阵容名称"`
	Entries []compactLineupEntryArguments `json:"entries" jsonschema:"required,description=非空首发条目"`
}
type gameCreateArguments struct {
	HomeTeamName string                 `json:"home_team_name"`
	AwayTeamName string                 `json:"away_team_name"`
	ScheduledAt  string                 `json:"scheduled_at"`
	Location     string                 `json:"location"`
	HomeLineup   compactLineupArguments `json:"home_lineup"`
	AwayLineup   compactLineupArguments `json:"away_lineup"`
	Plays        []compactPlayArguments `json:"plays"`
}

func objectModifyField(name, description string, required bool, fields ...modifyFieldDescription) modifyFieldDescription {
	return modifyFieldDescription{Name: name, Type: "object", Required: required, Description: description, Fields: fields}
}
func arrayModifyField(name, description string, required bool, item modifyFieldDescription) modifyFieldDescription {
	return modifyFieldDescription{Name: name, Type: "array", Required: required, Description: description, Item: &item}
}
func gameCreateFields() []modifyFieldDescription {
	playerRef := objectModifyField("player", "用精确球队名称和该队当前背号定位球员；不传内部 ID。", true, modifyFieldDescription{Name: "team_name", Type: "string", Required: true, Description: "比赛参与球队的精确名称。"}, modifyFieldDescription{Name: "jersey_number", Type: "integer", Required: true, Description: "该队当前背号，范围 0 到 99。"})
	lineupEntry := objectModifyField("entry", "首发条目；同一阵容中棒次和球员均不可重复。", true, playerRef, modifyFieldDescription{Name: "batting_order", Type: "integer", Required: true, Description: "首发棒次，范围 1 到 9。"}, modifyFieldDescription{Name: "position", Type: "string", Required: true, Description: "该条目的唯一守备位置。", Values: []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}})
	lineup := func(name string) modifyFieldDescription {
		return objectModifyField(name, "必填首发阵容；其中球员必须属于对应的主队或客队。", true, modifyFieldDescription{Name: "name", Type: "string", Required: true, Description: "非空阵容名称。"}, arrayModifyField("entries", "非空首发条目。", true, lineupEntry))
	}
	situation := func(name string) modifyFieldDescription {
		return objectModifyField(name, "该打席开始前或结束后的完整局面。", true, modifyFieldDescription{Name: "outs", Type: "integer", Required: true, Description: "当前半局累计出局数；before 只能为 0 到 2，after 可为 0 到 3。"}, modifyFieldDescription{Name: "home_score", Type: "integer", Required: true, Description: "该时点主队累计比分。"}, modifyFieldDescription{Name: "away_score", Type: "integer", Required: true, Description: "该时点客队累计比分。"}, arrayModifyField("runners", "固定三个元素，依次是一、二、三垒跑者；空垒为 null。", true, playerRef))
	}
	pitch := objectModifyField("pitch", "按实际投球顺序记录的逐球结果。", true, objectModifyField("pitcher", "该球实际投手；通常与 starting_pitcher 相同。", true, playerRef.Fields...), objectModifyField("batter", "该球打者；应与 Play 的 batter 对应。", true, playerRef.Fields...), modifyFieldDescription{Name: "result", Type: "string", Required: true, Description: "该球结果。决定打席结果的最后一球须符合 batting_result。", Values: []string{"ball", "called_strike", "swinging_strike", "foul", "foul_tip", "in_play", "hit_by_pitch", "intentional_ball", "pitchout", "other"}}, modifyFieldDescription{Name: "balls_before", Type: "integer", Required: true, Description: "投出前坏球数，范围 0 到 3。"}, modifyFieldDescription{Name: "strikes_before", Type: "integer", Required: true, Description: "投出前好球数，范围 0 到 2。"}, modifyFieldDescription{Name: "balls_after", Type: "integer", Required: true, Description: "投出后坏球数，范围 0 到 4。"}, modifyFieldDescription{Name: "strikes_after", Type: "integer", Required: true, Description: "投出后好球数，范围 0 到 3。"}, modifyFieldDescription{Name: "pitch_type", Type: "string", Required: false, Description: "球种；未知时省略。"}, modifyFieldDescription{Name: "velocity", Type: "number|null", Required: false, Description: "球速；未知时省略或 null，不能为负。"}, modifyFieldDescription{Name: "zone", Type: "integer|null", Required: false, Description: "进垒区域；未知时省略或 null，范围 0 到 255。"}, modifyFieldDescription{Name: "description", Type: "string", Required: false, Description: "该球的补充说明。"})
	runner := objectModifyField("runner_outcome", "本打席中单个跑者的结果；打者首次上垒使用 from_base=0。", true, objectModifyField("runner", "发生该结果的跑者或打者。", true, playerRef.Fields...), modifyFieldDescription{Name: "result", Type: "string", Required: true, Description: "跑垒结果类型。", Values: []string{"advance", "score", "force_out", "tag_out", "caught_stealing", "picked_off", "error_advance"}}, modifyFieldDescription{Name: "from_base", Type: "integer", Required: true, Description: "起始垒位：0 表示本打席打者尚未上垒，1 到 3 表示垒上跑者。"}, modifyFieldDescription{Name: "to_base", Type: "integer|null", Required: false, Description: "目标垒位，范围 1 到 4；得分通常填 4，出局时可省略。"}, modifyFieldDescription{Name: "out_recorded", Type: "boolean", Required: true, Description: "该跑者结果是否形成一个出局。"}, modifyFieldDescription{Name: "scored", Type: "boolean", Required: true, Description: "仅 result=score 时为 true；其余为 false。"}, objectModifyField("charged_pitcher", "责任投手；scored=true 时必填。", false, playerRef.Fields...), modifyFieldDescription{Name: "earned", Type: "boolean|null", Required: false, Description: "得分是否为自责分；scored=true 时必填。"}, objectModifyField("rbi_batter", "获得该分打点的打者；没有打点时省略。", false, playerRef.Fields...), modifyFieldDescription{Name: "description", Type: "string", Required: false, Description: "跑垒结果补充说明。"})
	fielding := objectModifyField("fielding_outcome", "本打席中单个守备员的守备结果，按发生顺序记录。", true, objectModifyField("fielder", "守备员。", true, playerRef.Fields...), modifyFieldDescription{Name: "position", Type: "string", Required: true, Description: "该守备结果发生时的唯一守备位置。", Values: []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}}, modifyFieldDescription{Name: "result", Type: "string", Required: true, Description: "守备结果类型。", Values: []string{"putout", "assist", "error", "double_play", "triple_play", "passed_ball", "catcher_interference", "other"}}, modifyFieldDescription{Name: "description", Type: "string", Required: false, Description: "守备结果补充说明。"})
	play := objectModifyField("play", "一个按数组顺序连续编号的打席；sequence 由数组位置生成。", true, modifyFieldDescription{Name: "inning", Type: "integer", Required: true, Description: "局数，从 1 开始。"}, modifyFieldDescription{Name: "half", Type: "string", Required: true, Description: "top 为客队进攻，bottom 为主队进攻。", Values: []string{"top", "bottom"}}, modifyFieldDescription{Name: "batting_order", Type: "integer", Required: true, Description: "本打席打者的棒次，范围 1 到 9。"}, objectModifyField("batter", "本打席打者。", true, playerRef.Fields...), objectModifyField("starting_pitcher", "打席开始时的投手。", true, playerRef.Fields...), situation("before"), situation("after"), modifyFieldDescription{Name: "batting_result", Type: "string", Required: true, Description: "该打席的最终打击结果。", Values: []string{"single", "double", "triple", "home_run", "walk", "intentional_walk", "hit_by_pitch", "strikeout", "ground_out", "fly_out", "line_out", "fielders_choice", "reached_on_error", "sacrifice_bunt", "sacrifice_fly", "interference", "other"}}, modifyFieldDescription{Name: "result_description", Type: "string", Required: true, Description: "非空的本打席结果文字说明。"}, arrayModifyField("pitches", "至少一颗、按实际顺序排列的投球。", true, pitch), arrayModifyField("runner_outcomes", "本打席发生的跑垒结果；没有跑垒变化时可省略或传空数组。", false, runner), arrayModifyField("fielding_outcomes", "本打席发生的守备结果；没有可记录守备事实时可省略或传空数组。", false, fielding))
	return []modifyFieldDescription{{Name: "home_team_name", Type: "string", Required: true, Description: "唯一主队的精确名称。"}, {Name: "away_team_name", Type: "string", Required: true, Description: "唯一客队的精确名称，必须与主队不同。"}, {Name: "scheduled_at", Type: "string", Required: true, Description: "RFC3339 比赛时间。"}, {Name: "location", Type: "string", Required: true, Description: "比赛地点。"}, lineup("home_lineup"), lineup("away_lineup"), arrayModifyField("plays", "非空、完整且连续的比赛过程。", true, play)}
}

func gameCreateConventions() []string {
	return []string{
		"每个 Play 都显式给出 before 和 after；同一半局的相邻 Play 将前一 after 作为后一 before。换半局时，前一 after 的 outs 为 3，下一 before 从 outs=0、空垒开始，比分保持不变。",
		"runner_outcomes 记录本打席所有可确定的跑垒变化。打者安全上垒按跑者处理，使用 from_base=0；一垒安打通常为 advance 到 1，阳春本垒打通常为 score 到 4。",
		"得分跑者使用 result=score、scored=true，并填写 charged_pitcher 与 earned；有打点时填写 rbi_batter。after 的比分增量必须等于本 Play 中 scored=true 的结果数。",
		"fielding_outcomes 逐守备员记录 assist、putout、error 等事实。双杀通常记录参与者各自的 assist 或 putout，必要时再补充 double_play 标记，而不以文字说明替代守备事实。",
		"pitches 按实际投球顺序填写；未知的 pitch_type、velocity、zone 可以省略。最后一球须与 batting_result 相容：保送以 ball 或 intentional_ball 结束，三振以 called_strike、swinging_strike 或 foul_tip 结束，其他结果以 in_play 或 other 结束。",
	}
}

func gameCreateInvariants() []string {
	return []string{
		"球队、球员引用、两套首发阵容与全部 Play 会先完成解析和领域校验，之后才开始写入。",
		"每个 Play 的 sequence 由 plays 数组位置生成，必须连续；每套首发阵容非空，且棒次、球员和守备位置满足领域规则。",
		"局面中的跑者必须唯一；before.outs 范围为 0 到 2，after.outs 范围为 0 到 3，比分不能倒退。",
		"每个 Play 至少有一颗 Pitch，子项顺序连续；最后一颗 Pitch 必须能结束对应的 batting_result。",
		"runner_outcome 的 from_base 范围为 0 到 3，to_base（如提供）范围为 1 到 4；scored 与 result=score 一致，得分时 charged_pitcher 和 earned 必填。",
		"game.create 不使用数据库事务。若底层写入阶段失败，返回 partial，已经写入的比赛、阵容或 Play 可能保留。",
	}
}

func compactGameCreateFields() []modifyFieldDescription {
	lineupEntry := objectModifyField("entry", "首发球员；球队由所在阵容推导。", true,
		modifyFieldDescription{Name: "jersey_number", Type: "integer", Required: true, Description: "该阵容球队中的当前背号，范围 0 到 99。"},
		modifyFieldDescription{Name: "batting_order", Type: "integer", Required: true, Description: "首发棒次，范围 1 到 9。"},
		modifyFieldDescription{Name: "position", Type: "string", Required: true, Description: "唯一守备位置。", Values: []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}})
	lineup := func(name string) modifyFieldDescription {
		return objectModifyField(name, "首发阵容；球队由字段名推导。", true, modifyFieldDescription{Name: "name", Type: "string", Required: true, Description: "阵容名称。"}, arrayModifyField("entries", "非空首发条目。", true, lineupEntry))
	}
	pitch := objectModifyField("pitch", "逐球结果；服务端从顺序推导好坏球数。", true, modifyFieldDescription{Name: "result", Type: "string", Required: true, Description: "投球结果。", Values: []string{"ball", "called_strike", "swinging_strike", "foul", "foul_tip", "in_play", "hit_by_pitch", "intentional_ball", "pitchout", "other"}}, modifyFieldDescription{Name: "pitch_type", Type: "string", Required: false, Description: "球种，未知时省略。"}, modifyFieldDescription{Name: "velocity", Type: "number", Required: false, Description: "非负球速，未知时省略。"}, modifyFieldDescription{Name: "zone", Type: "integer", Required: false, Description: "0 到 255 的进垒区域，未知时省略。"}, modifyFieldDescription{Name: "description", Type: "string", Required: false, Description: "补充说明。"})
	runner := objectModifyField("runner_outcome", "发生变化的进攻方跑者；服务端推导得分和出局标记。", true, modifyFieldDescription{Name: "jersey_number", Type: "integer", Required: true, Description: "进攻方跑者背号；from_base=0 时必须是打者。"}, modifyFieldDescription{Name: "result", Type: "string", Required: true, Description: "跑垒结果。", Values: []string{"advance", "score", "force_out", "tag_out", "caught_stealing", "picked_off", "error_advance"}}, modifyFieldDescription{Name: "from_base", Type: "integer", Required: true, Description: "0 表示打者，1 到 3 表示当前垒位。"}, modifyFieldDescription{Name: "to_base", Type: "integer", Required: false, Description: "非得分安全上垒的目标垒位 1 到 3。"}, modifyFieldDescription{Name: "charged_pitcher_jersey_number", Type: "integer", Required: false, Description: "得分责任投手；省略时使用本打席投手。"}, modifyFieldDescription{Name: "earned", Type: "boolean", Required: false, Description: "得分是否自责；result=score 时必填。"}, modifyFieldDescription{Name: "rbi_batter_jersey_number", Type: "integer", Required: false, Description: "获得打点的进攻方打者背号。"}, modifyFieldDescription{Name: "description", Type: "string", Required: false, Description: "补充说明。"})
	fielding := objectModifyField("fielding_outcome", "守备方的守备事实；球队由 half 推导。", true, modifyFieldDescription{Name: "jersey_number", Type: "integer", Required: true, Description: "守备球员背号。"}, modifyFieldDescription{Name: "position", Type: "string", Required: true, Description: "唯一守备位置。", Values: []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}}, modifyFieldDescription{Name: "result", Type: "string", Required: true, Description: "守备结果。", Values: []string{"putout", "assist", "error", "double_play", "triple_play", "passed_ball", "catcher_interference", "other"}}, modifyFieldDescription{Name: "description", Type: "string", Required: false, Description: "补充说明。"})
	play := objectModifyField("play", "连续打席；服务端依顺序推导局面。", true, modifyFieldDescription{Name: "inning", Type: "integer", Required: true, Description: "局数，从 1 开始。"}, modifyFieldDescription{Name: "half", Type: "string", Required: true, Description: "top 为客队进攻，bottom 为主队进攻。", Values: []string{"top", "bottom"}}, modifyFieldDescription{Name: "batting_order", Type: "integer", Required: true, Description: "打者棒次。"}, modifyFieldDescription{Name: "batter_jersey_number", Type: "integer", Required: true, Description: "进攻方打者背号。"}, modifyFieldDescription{Name: "pitcher_jersey_number", Type: "integer", Required: true, Description: "守备方本打席投手背号。"}, modifyFieldDescription{Name: "batting_result", Type: "string", Required: true, Description: "打席最终结果。", Values: []string{"single", "double", "triple", "home_run", "walk", "intentional_walk", "hit_by_pitch", "strikeout", "ground_out", "fly_out", "line_out", "fielders_choice", "reached_on_error", "sacrifice_bunt", "sacrifice_fly", "interference", "other"}}, modifyFieldDescription{Name: "result_description", Type: "string", Required: true, Description: "非空的结果文字说明。"}, arrayModifyField("pitches", "至少一颗，按实际顺序。", true, pitch), arrayModifyField("runner_outcomes", "本打席发生变化的跑者。", false, runner), arrayModifyField("fielding_outcomes", "守备事实。", false, fielding))
	return []modifyFieldDescription{{Name: "home_team_name", Type: "string", Required: true, Description: "主队精确名称。"}, {Name: "away_team_name", Type: "string", Required: true, Description: "客队精确名称。"}, {Name: "scheduled_at", Type: "string", Required: true, Description: "RFC3339 比赛时间。"}, {Name: "location", Type: "string", Required: true, Description: "比赛地点。"}, lineup("home_lineup"), lineup("away_lineup"), arrayModifyField("plays", "完整连续的比赛过程。", true, play)}
}
func compactGameCreateConventions() []string {
	return []string{"每个 Play 只写事件事实；球队、好坏球数、局面、比分、得分和出局由服务端按数组顺序推导。", "top 为客队进攻，bottom 为主队进攻；换半局前必须已有三出局。", "runner_outcomes 只列发生变化的跑者；from_base=0 表示打者。得分必须给 earned，责任投手默认本打席 pitcher。", "pitches 顺序决定球数：坏球增加 balls，好球和两好球前界外球增加 strikes。"}
}
func compactGameCreateInvariants() []string {
	return []string{"首个 Play 必须是一局上半；同半局连续，换半局需要三出局。", "打席至少一球；打席结果和跑垒/守备事实必须与推导的局面相容。", "球队、球员和阵容在写入前统一解析与校验；写入阶段失败仍返回 partial，已写内容可能保留。"}
}

func requiredNamedInt(value *int, name string) (int, error) {
	if value == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	return *value, nil
}
func requiredNamedBool(value *bool, name string) (bool, error) {
	if value == nil {
		return false, fmt.Errorf("%s is required", name)
	}
	return *value, nil
}
func convertPlayerReference(value namedPlayerArguments) (game.PlayerReference, error) {
	jersey, err := requiredNamedInt(value.JerseyNumber, "jersey_number")
	if err != nil {
		return game.PlayerReference{}, err
	}
	if jersey < 0 || jersey > 99 || strings.TrimSpace(value.TeamName) == "" {
		return game.PlayerReference{}, errors.New("player team_name and jersey_number 0..99 are required")
	}
	return game.PlayerReference{TeamName: strings.TrimSpace(value.TeamName), JerseyNumber: jersey}, nil
}
func convertOptionalReference(value *namedPlayerArguments) (*game.PlayerReference, error) {
	if value == nil {
		return nil, nil
	}
	converted, err := convertPlayerReference(*value)
	return &converted, err
}
func convertSituation(value namedSituationArguments) (game.NamedSituationDraft, error) {
	outs, e := requiredNamedInt(value.Outs, "outs")
	if e != nil {
		return game.NamedSituationDraft{}, e
	}
	home, e := requiredNamedInt(value.HomeScore, "home_score")
	if e != nil {
		return game.NamedSituationDraft{}, e
	}
	away, e := requiredNamedInt(value.AwayScore, "away_score")
	if e != nil {
		return game.NamedSituationDraft{}, e
	}
	if len(value.Runners) != 3 {
		return game.NamedSituationDraft{}, errors.New("runners must contain exactly three entries")
	}
	result := game.NamedSituationDraft{Outs: outs, HomeScore: home, AwayScore: away}
	for i, ref := range value.Runners {
		if ref == nil {
			continue
		}
		converted, e := convertPlayerReference(*ref)
		if e != nil {
			return game.NamedSituationDraft{}, e
		}
		result.Runners[i] = &converted
	}
	return result, nil
}
func enumValue[T comparable](name, value string, values map[string]T) (T, error) {
	converted, ok := values[value]
	if !ok {
		var zero T
		return zero, fmt.Errorf("unknown %s %q", name, value)
	}
	return converted, nil
}
func convertNamedGame(value legacyGameCreateArguments) (game.NamedGameRecordDraft, error) {
	result := game.NamedGameRecordDraft{HomeTeamName: strings.TrimSpace(value.HomeTeamName), AwayTeamName: strings.TrimSpace(value.AwayTeamName), ScheduledAt: strings.TrimSpace(value.ScheduledAt), Location: value.Location}
	if result.HomeTeamName == "" || result.AwayTeamName == "" || result.ScheduledAt == "" {
		return result, errors.New("home_team_name, away_team_name and scheduled_at are required")
	}
	convertLineup := func(input namedLineupArguments) (game.NamedLineupDraft, error) {
		out := game.NamedLineupDraft{Name: input.Name}
		for _, entry := range input.Entries {
			ref, e := convertPlayerReference(entry.Player)
			if e != nil {
				return out, e
			}
			order, e := requiredNamedInt(entry.BattingOrder, "batting_order")
			if e != nil {
				return out, e
			}
			pos, e := parsePositions([]string{entry.Position})
			if e != nil {
				return out, e
			}
			out.Entries = append(out.Entries, game.NamedLineupEntryDraft{Player: ref, BattingOrder: order, Position: pos})
		}
		return out, nil
	}
	var err error
	result.HomeLineup, err = convertLineup(value.HomeLineup)
	if err != nil {
		return result, err
	}
	result.AwayLineup, err = convertLineup(value.AwayLineup)
	if err != nil {
		return result, err
	}
	pitchResults := map[string]game.PitchResult{"ball": game.PitchBall, "called_strike": game.PitchCalledStrike, "swinging_strike": game.PitchSwingingStrike, "foul": game.PitchFoul, "foul_tip": game.PitchFoulTip, "in_play": game.PitchInPlay, "hit_by_pitch": game.PitchHitByPitch, "intentional_ball": game.PitchIntentionalBall, "pitchout": game.PitchPitchout, "other": game.PitchOther}
	battingResults := map[string]game.BattingResult{"single": game.BattingSingle, "double": game.BattingDouble, "triple": game.BattingTriple, "home_run": game.BattingHomeRun, "walk": game.BattingWalk, "intentional_walk": game.BattingIntentionalWalk, "hit_by_pitch": game.BattingHitByPitch, "strikeout": game.BattingStrikeout, "ground_out": game.BattingGroundOut, "fly_out": game.BattingFlyOut, "line_out": game.BattingLineOut, "fielders_choice": game.BattingFieldersChoice, "reached_on_error": game.BattingReachedOnError, "sacrifice_bunt": game.BattingSacrificeBunt, "sacrifice_fly": game.BattingSacrificeFly, "interference": game.BattingInterference, "other": game.BattingOther}
	runnerResults := map[string]game.RunnerResult{"advance": game.RunnerAdvance, "score": game.RunnerScore, "force_out": game.RunnerForceOut, "tag_out": game.RunnerTagOut, "caught_stealing": game.RunnerCaughtStealing, "picked_off": game.RunnerPickedOff, "error_advance": game.RunnerErrorAdvance}
	fieldingResults := map[string]game.FieldingResult{"putout": game.FieldingPutout, "assist": game.FieldingAssist, "error": game.FieldingError, "double_play": game.FieldingDoublePlay, "triple_play": game.FieldingTriplePlay, "passed_ball": game.FieldingPassedBall, "catcher_interference": game.FieldingCatcherInterference, "other": game.FieldingOther}
	for _, input := range value.Plays {
		inning, e := requiredNamedInt(input.Inning, "inning")
		if e != nil {
			return result, e
		}
		order, e := requiredNamedInt(input.BattingOrder, "batting_order")
		if e != nil {
			return result, e
		}
		half, e := parseHalf(input.Half)
		if e != nil {
			return result, e
		}
		batter, e := convertPlayerReference(input.Batter)
		if e != nil {
			return result, e
		}
		starter, e := convertPlayerReference(input.StartingPitcher)
		if e != nil {
			return result, e
		}
		before, e := convertSituation(input.Before)
		if e != nil {
			return result, e
		}
		after, e := convertSituation(input.After)
		if e != nil {
			return result, e
		}
		batResult, e := enumValue("batting_result", input.BattingResult, battingResults)
		if e != nil {
			return result, e
		}
		play := game.NamedPlayDraft{Inning: inning, BattingOrder: order, Half: half, Batter: batter, StartingPitcher: starter, Before: before, After: after, BattingResult: batResult, ResultDescription: input.ResultDescription}
		for _, p := range input.Pitches {
			pitcher, e := convertPlayerReference(p.Pitcher)
			if e != nil {
				return result, e
			}
			pbatter, e := convertPlayerReference(p.Batter)
			if e != nil {
				return result, e
			}
			pres, e := enumValue("pitch result", p.Result, pitchResults)
			if e != nil {
				return result, e
			}
			bb, e := requiredNamedInt(p.BallsBefore, "balls_before")
			if e != nil {
				return result, e
			}
			sb, e := requiredNamedInt(p.StrikesBefore, "strikes_before")
			if e != nil {
				return result, e
			}
			ba, e := requiredNamedInt(p.BallsAfter, "balls_after")
			if e != nil {
				return result, e
			}
			sa, e := requiredNamedInt(p.StrikesAfter, "strikes_after")
			if e != nil {
				return result, e
			}
			play.Pitches = append(play.Pitches, game.NamedPitchDraft{Pitcher: pitcher, Batter: pbatter, Result: pres, BallsBefore: bb, StrikesBefore: sb, BallsAfter: ba, StrikesAfter: sa, PitchType: p.PitchType, Velocity: p.Velocity, Zone: p.Zone, Description: p.Description})
		}
		for _, r := range input.RunnerOutcomes {
			runner, e := convertPlayerReference(r.Runner)
			if e != nil {
				return result, e
			}
			rr, e := enumValue("runner result", r.Result, runnerResults)
			if e != nil {
				return result, e
			}
			from, e := requiredNamedInt(r.FromBase, "from_base")
			if e != nil {
				return result, e
			}
			outRecorded, e := requiredNamedBool(r.OutRecorded, "out_recorded")
			if e != nil {
				return result, e
			}
			scored, e := requiredNamedBool(r.Scored, "scored")
			if e != nil {
				return result, e
			}
			charged, e := convertOptionalReference(r.ChargedPitcher)
			if e != nil {
				return result, e
			}
			rbi, e := convertOptionalReference(r.RBIBatter)
			if e != nil {
				return result, e
			}
			play.RunnerOutcomes = append(play.RunnerOutcomes, game.NamedRunnerOutcomeDraft{Runner: runner, Result: rr, FromBase: from, ToBase: r.ToBase, OutRecorded: outRecorded, Scored: scored, ChargedPitcher: charged, Earned: r.Earned, RBIBatter: rbi, Description: r.Description})
		}
		for _, f := range input.FieldingOutcomes {
			fielder, e := convertPlayerReference(f.Fielder)
			if e != nil {
				return result, e
			}
			pos, e := parsePositions([]string{f.Position})
			if e != nil {
				return result, e
			}
			fr, e := enumValue("fielding result", f.Result, fieldingResults)
			if e != nil {
				return result, e
			}
			play.FieldingOutcomes = append(play.FieldingOutcomes, game.NamedFieldingOutcomeDraft{Fielder: fielder, Position: pos, Result: fr, Description: f.Description})
		}
		result.Plays = append(result.Plays, play)
	}
	if len(result.Plays) == 0 {
		return result, errors.New("plays are required")
	}
	return result, nil
}

func convertCompactGame(value gameCreateArguments) (game.CompactGameRecordDraft, error) {
	result := game.CompactGameRecordDraft{HomeTeamName: strings.TrimSpace(value.HomeTeamName), AwayTeamName: strings.TrimSpace(value.AwayTeamName), ScheduledAt: strings.TrimSpace(value.ScheduledAt), Location: value.Location}
	if result.HomeTeamName == "" || result.AwayTeamName == "" || result.ScheduledAt == "" {
		return result, errors.New("home_team_name, away_team_name and scheduled_at are required")
	}
	lineup := func(input compactLineupArguments) (game.CompactLineupDraft, error) {
		out := game.CompactLineupDraft{Name: input.Name}
		for _, entry := range input.Entries {
			jersey, err := requiredNamedInt(entry.JerseyNumber, "jersey_number")
			if err != nil {
				return out, err
			}
			order, err := requiredNamedInt(entry.BattingOrder, "batting_order")
			if err != nil {
				return out, err
			}
			position, err := parsePositions([]string{entry.Position})
			if err != nil {
				return out, err
			}
			out.Entries = append(out.Entries, game.CompactLineupEntryDraft{JerseyNumber: jersey, BattingOrder: order, Position: position})
		}
		return out, nil
	}
	var err error
	if result.HomeLineup, err = lineup(value.HomeLineup); err != nil {
		return result, err
	}
	if result.AwayLineup, err = lineup(value.AwayLineup); err != nil {
		return result, err
	}
	pitchResults := map[string]game.PitchResult{"ball": game.PitchBall, "called_strike": game.PitchCalledStrike, "swinging_strike": game.PitchSwingingStrike, "foul": game.PitchFoul, "foul_tip": game.PitchFoulTip, "in_play": game.PitchInPlay, "hit_by_pitch": game.PitchHitByPitch, "intentional_ball": game.PitchIntentionalBall, "pitchout": game.PitchPitchout, "other": game.PitchOther}
	battingResults := map[string]game.BattingResult{"single": game.BattingSingle, "double": game.BattingDouble, "triple": game.BattingTriple, "home_run": game.BattingHomeRun, "walk": game.BattingWalk, "intentional_walk": game.BattingIntentionalWalk, "hit_by_pitch": game.BattingHitByPitch, "strikeout": game.BattingStrikeout, "ground_out": game.BattingGroundOut, "fly_out": game.BattingFlyOut, "line_out": game.BattingLineOut, "fielders_choice": game.BattingFieldersChoice, "reached_on_error": game.BattingReachedOnError, "sacrifice_bunt": game.BattingSacrificeBunt, "sacrifice_fly": game.BattingSacrificeFly, "interference": game.BattingInterference, "other": game.BattingOther}
	runnerResults := map[string]game.RunnerResult{"advance": game.RunnerAdvance, "score": game.RunnerScore, "force_out": game.RunnerForceOut, "tag_out": game.RunnerTagOut, "caught_stealing": game.RunnerCaughtStealing, "picked_off": game.RunnerPickedOff, "error_advance": game.RunnerErrorAdvance}
	fieldingResults := map[string]game.FieldingResult{"putout": game.FieldingPutout, "assist": game.FieldingAssist, "error": game.FieldingError, "double_play": game.FieldingDoublePlay, "triple_play": game.FieldingTriplePlay, "passed_ball": game.FieldingPassedBall, "catcher_interference": game.FieldingCatcherInterference, "other": game.FieldingOther}
	for _, input := range value.Plays {
		inning, err := requiredNamedInt(input.Inning, "inning")
		if err != nil {
			return result, err
		}
		order, err := requiredNamedInt(input.BattingOrder, "batting_order")
		if err != nil {
			return result, err
		}
		batter, err := requiredNamedInt(input.BatterJerseyNumber, "batter_jersey_number")
		if err != nil {
			return result, err
		}
		pitcher, err := requiredNamedInt(input.PitcherJerseyNumber, "pitcher_jersey_number")
		if err != nil {
			return result, err
		}
		half, err := parseHalf(input.Half)
		if err != nil {
			return result, err
		}
		batting, err := enumValue("batting_result", input.BattingResult, battingResults)
		if err != nil {
			return result, err
		}
		play := game.CompactPlayDraft{Inning: inning, Half: half, BattingOrder: order, BatterJerseyNumber: batter, PitcherJerseyNumber: pitcher, BattingResult: batting, ResultDescription: input.ResultDescription}
		for _, inputPitch := range input.Pitches {
			pitchResult, err := enumValue("pitch result", inputPitch.Result, pitchResults)
			if err != nil {
				return result, err
			}
			play.Pitches = append(play.Pitches, game.CompactPitchDraft{Result: pitchResult, PitchType: inputPitch.PitchType, Velocity: inputPitch.Velocity, Zone: inputPitch.Zone, Description: inputPitch.Description})
		}
		for _, inputRunner := range input.RunnerOutcomes {
			jersey, err := requiredNamedInt(inputRunner.JerseyNumber, "jersey_number")
			if err != nil {
				return result, err
			}
			from, err := requiredNamedInt(inputRunner.FromBase, "from_base")
			if err != nil {
				return result, err
			}
			runnerResult, err := enumValue("runner result", inputRunner.Result, runnerResults)
			if err != nil {
				return result, err
			}
			play.RunnerOutcomes = append(play.RunnerOutcomes, game.CompactRunnerOutcomeDraft{JerseyNumber: jersey, Result: runnerResult, FromBase: from, ToBase: inputRunner.ToBase, ChargedPitcherJerseyNumber: inputRunner.ChargedPitcherJerseyNumber, Earned: inputRunner.Earned, RBIBatterJerseyNumber: inputRunner.RBIBatterJerseyNumber, Description: inputRunner.Description})
		}
		for _, inputFielding := range input.FieldingOutcomes {
			jersey, err := requiredNamedInt(inputFielding.JerseyNumber, "jersey_number")
			if err != nil {
				return result, err
			}
			position, err := parsePositions([]string{inputFielding.Position})
			if err != nil {
				return result, err
			}
			fielding, err := enumValue("fielding result", inputFielding.Result, fieldingResults)
			if err != nil {
				return result, err
			}
			play.FieldingOutcomes = append(play.FieldingOutcomes, game.CompactFieldingOutcomeDraft{JerseyNumber: jersey, Position: position, Result: fielding, Description: inputFielding.Description})
		}
		result.Plays = append(result.Plays, play)
	}
	if len(result.Plays) == 0 {
		return result, errors.New("plays are required")
	}
	return result, nil
}

func matchBaseFields() []modifyFieldDescription {
	return []modifyFieldDescription{{Name: "home_team_id", Type: "string", Required: true, Description: "主队 ID。"}, {Name: "away_team_id", Type: "string", Required: true, Description: "客队 ID。"}, {Name: "scheduled_at", Type: "string", Required: true, Description: "RFC3339 比赛时间。"}, {Name: "location", Type: "string", Required: true, Description: "比赛地点。"}}
}
func matchCreateFields() []modifyFieldDescription {
	return append(matchBaseFields(), modifyFieldDescription{Name: "status", Type: "string", Required: true, Description: "比赛状态。", Values: []string{"scheduled", "in_progress", "final", "cancelled"}})
}
func lineupKeyFields() []modifyFieldDescription {
	return []modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}, {Name: "team_id", Type: "string", Required: true, Description: "球队 ID。"}, {Name: "kind", Type: "string", Required: true, Description: "阵容种类。", Values: []string{"starter", "backup"}}, {Name: "variant_number", Type: "integer", Required: true, Description: "首发为 0，候选阵容为正数。"}}
}
func lineupFields() []modifyFieldDescription {
	return append(lineupKeyFields(), modifyFieldDescription{Name: "variant_name", Type: "string", Required: true, Description: "阵容名称。"}, modifyFieldDescription{Name: "entries", Type: "array<lineup_entry>", Required: true, Description: "球员、棒次和单一守备位置列表。"})
}

func parseStatus(v string) (game.MatchStatus, error) {
	values := map[string]game.MatchStatus{"scheduled": game.MatchScheduled, "in_progress": game.MatchInProgress, "final": game.MatchFinal, "cancelled": game.MatchCancelled}
	x, ok := values[v]
	if !ok {
		return 0, fmt.Errorf("unknown match status %q", v)
	}
	return x, nil
}
func parseLineupKind(v string) (game.LineupKind, error) {
	if v == "starter" {
		return game.LineupStarter, nil
	}
	if v == "backup" {
		return game.LineupBackup, nil
	}
	return 0, fmt.Errorf("unknown lineup kind %q", v)
}
func parseHalf(v string) (game.Half, error) {
	if v == "top" {
		return game.Top, nil
	}
	if v == "bottom" {
		return game.Bottom, nil
	}
	return 0, fmt.Errorf("unknown inning half %q", v)
}
func requireInt(v *int, name string) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	return *v, nil
}
func parseScheduled(v string) (time.Time, error) {
	x, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse scheduled_at: %w", err)
	}
	return x, nil
}
func parseLineupEntries(values []lineupEntryArguments, newID IDGenerator) ([]game.LineupEntry, error) {
	if len(values) == 0 {
		return nil, errors.New("entries are required")
	}
	result := make([]game.LineupEntry, 0, len(values))
	for _, v := range values {
		order, err := requireInt(v.BattingOrder, "batting_order")
		if err != nil {
			return nil, err
		}
		position, err := parsePositions([]string{v.Position})
		if err != nil {
			return nil, err
		}
		entry, err := game.NewLineupEntry(game.LineupID(newID()), player.ID(v.PlayerID), order, position)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, nil
}

func validateGameArguments(operation string, arguments map[string]any) error {
	switch operation {
	case "match.create":
		var v matchCreateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		_, err := parseStatus(v.Status)
		if err != nil {
			return err
		}
		_, err = parseScheduled(v.ScheduledAt)
		return err
	case "match.update":
		var v matchUpdateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		_, err := parseScheduled(v.ScheduledAt)
		return err
	case "match.set_status":
		var v matchStatusArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		_, err := parseStatus(v.Status)
		return err
	case "match.delete":
		return decodeArguments(arguments, &matchDeleteArguments{})
	case "lineup.create", "lineup.replace":
		var v lineupArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		if _, err := parseLineupKind(v.Kind); err != nil {
			return err
		}
		_, err := requireInt(v.VariantNumber, "variant_number")
		if err != nil {
			return err
		}
		index := 0
		_, err = parseLineupEntries(v.Entries, func() string { index++; return fmt.Sprintf("validation-id-%d", index) })
		return err
	case "lineup.delete":
		var v lineupDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		if _, err := parseLineupKind(v.Kind); err != nil {
			return err
		}
		_, err := requireInt(v.VariantNumber, "variant_number")
		return err
	default:
		return fmt.Errorf("unknown team modify operation %q", operation)
	}
}

func (h *teamModifyHandler) executeGame(ctx context.Context, operation string, arguments map[string]any) (any, error) {
	switch operation {
	case "match.create":
		var v matchCreateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		scheduled, err := parseScheduled(v.ScheduledAt)
		if err != nil {
			return nil, err
		}
		status, err := parseStatus(v.Status)
		if err != nil {
			return nil, err
		}
		value, err := h.games.CreateMatchWith(ctx, game.MatchID(h.newID()), team.ID(v.HomeTeamID), team.ID(v.AwayTeamID), scheduled, v.Location, status)
		if err != nil {
			return nil, err
		}
		return matchResult(value), nil
	case "match.update":
		var v matchUpdateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		scheduled, err := parseScheduled(v.ScheduledAt)
		if err != nil {
			return nil, err
		}
		value, err := h.games.UpdateMatch(ctx, game.MatchID(v.MatchID), team.ID(v.HomeTeamID), team.ID(v.AwayTeamID), scheduled, v.Location)
		if err != nil {
			return nil, err
		}
		return matchResult(value), nil
	case "match.set_status":
		var v matchStatusArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		status, err := parseStatus(v.Status)
		if err != nil {
			return nil, err
		}
		value, err := h.games.SetMatchStatus(ctx, game.MatchID(v.MatchID), status)
		if err != nil {
			return nil, err
		}
		return matchResult(value), nil
	case "match.delete":
		var v matchDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		if err := h.games.DeleteMatch(ctx, game.MatchID(v.MatchID)); err != nil {
			return nil, err
		}
		return map[string]any{"match_id": v.MatchID, "deleted": true}, nil
	case "lineup.create", "lineup.replace":
		var v lineupArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		kind, err := parseLineupKind(v.Kind)
		if err != nil {
			return nil, err
		}
		number, err := requireInt(v.VariantNumber, "variant_number")
		if err != nil {
			return nil, err
		}
		entries, err := parseLineupEntries(v.Entries, h.newID)
		if err != nil {
			return nil, err
		}
		var value game.Lineup
		if operation == "lineup.create" {
			value, err = h.games.CreateLineupWith(ctx, game.MatchID(v.MatchID), team.ID(v.TeamID), kind, number, v.VariantName, entries)
		} else {
			value, err = h.games.ReplaceLineupWith(ctx, game.MatchID(v.MatchID), team.ID(v.TeamID), kind, number, v.VariantName, entries)
		}
		if err != nil {
			return nil, err
		}
		return lineupResult(value), nil
	case "lineup.delete":
		var v lineupDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		kind, err := parseLineupKind(v.Kind)
		if err != nil {
			return nil, err
		}
		number, err := requireInt(v.VariantNumber, "variant_number")
		if err != nil {
			return nil, err
		}
		if err := h.games.DeleteLineup(ctx, game.MatchID(v.MatchID), team.ID(v.TeamID), kind, uint16(number)); err != nil {
			return nil, err
		}
		return map[string]any{"match_id": v.MatchID, "team_id": v.TeamID, "deleted": true}, nil
	default:
		return nil, fmt.Errorf("unknown team modify operation %q", operation)
	}
}
func matchResult(v game.Match) map[string]any {
	return map[string]any{"id": v.ID(), "home_team_id": v.HomeTeamID(), "away_team_id": v.AwayTeamID(), "scheduled_at": v.ScheduledAt().Format(time.RFC3339), "location": v.Location(), "status": matchStatusResult(v.Status())}
}
func matchStatusResult(v game.MatchStatus) string {
	values := map[game.MatchStatus]string{game.MatchScheduled: "scheduled", game.MatchInProgress: "in_progress", game.MatchFinal: "final", game.MatchCancelled: "cancelled"}
	return values[v]
}
func lineupResult(v game.Lineup) map[string]any {
	return map[string]any{"match_id": v.MatchID(), "team_id": v.TeamID(), "kind": v.Kind(), "variant_number": v.VariantNumber(), "variant_name": v.VariantName()}
}
