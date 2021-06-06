package v1

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	redis "gopkg.in/redis.v5"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"zxq.co/ripple/ocl"
)

type SleaderboardUser struct {
	userData
	ChosenMode    modeData `json:"chosen_mode"`
	PlayStyle     int      `json:"play_style"`
	FavouriteMode int      `json:"favourite_mode"`
}

type SleaderboardResponse struct {
	common.ResponseBase
	Users []SleaderboardUser `json:"users"`
}

const SrxUserQuery = `
		SELECT
			users.id, users.username, users.register_datetime, users.privileges, users.latest_activity,

			rx_stats.username_aka, users_stats.country,
			rx_stats.play_style, rx_stats.favourite_mode,

			rx_stats.ranked_score_%[1]s, rx_stats.total_score_%[1]s, rx_stats.playcount_%[1]s,
			rx_stats.replays_watched_%[1]s, rx_stats.total_hits_%[1]s,
			rx_stats.avg_accuracy_%[1]s
		FROM users
		INNER JOIN rx_stats ON rx_stats.id = users.id
		INNER JOIN users_stats ON users_stats.id = users.id
		WHERE users.id IN (?)
		`

const SlbUserQuery = `
		SELECT
			users.id, users.username, users.register_datetime, users.privileges, users.latest_activity,

			users_stats.username_aka, users_stats.country,
			users_stats.play_style, users_stats.favourite_mode,

			users_stats.ranked_score_%[1]s, users_stats.total_score_%[1]s, users_stats.playcount_%[1]s,
			users_stats.replays_watched_%[1]s, users_stats.total_hits_%[1]s,
			users_stats.avg_accuracy_%[1]s
		FROM users
		INNER JOIN users_stats ON users_stats.id = users.id
		WHERE users.id IN (?)
		`

// LeaderboardGET gets the leaderboard.
func SLeaderboardGET(md common.MethodData) common.CodeMessager {
	m := getMode(md.Query("mode"))

	// md.Query.Country
	p := common.Int(md.Query("p")) - 1
	if p < 0 {
		p = 0
	}
	l := common.InString(1, md.Query("l"), 500, 50)

	key := "ripple:leaderboard:" + m
	if common.Int(md.Query("rx")) != 0 {
		key = "ripple:relaxboard:" + m
	}
	if md.Query("country") != "" {
		key += ":" + md.Query("country")
	}

	results, err := md.R.ZRevRange(key, int64(p*l), int64(p*l+l-1)).Result()
	if err != nil {
		md.Err(err)
		return Err500
	}

	var resp SleaderboardResponse
	resp.Code = 200

	if len(results) == 0 {
		return resp
	}

	query := fmt.Sprintf(lbUserQuery+` ORDER BY users_stats.ranked_score_%[1]s DESC`, m)
	if common.Int(md.Query("rx")) != 0 {
		query = fmt.Sprintf(rxUserQuery+` ORDER BY rx_stats.ranked_score_%[1]s DESC`, m)
	}
	query, params, _ := sqlx.In(query, results)
	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	for rows.Next() {
		var u SleaderboardUser
		err := rows.Scan(
			&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges, &u.LatestActivity,

			&u.UsernameAKA, &u.Country, &u.PlayStyle, &u.FavouriteMode,

			&u.ChosenMode.RankedScore, &u.ChosenMode.TotalScore, &u.ChosenMode.PlayCount,
			&u.ChosenMode.ReplaysWatched, &u.ChosenMode.TotalHits,
			&u.ChosenMode.Accuracy, &u.ChosenMode.PP,
		)
		if err != nil {
			md.Err(err)
			continue
		}
		u.ChosenMode.Level = ocl.GetLevelPrecise(int64(u.ChosenMode.TotalScore))
		if common.Int(md.Query("rx")) != 0 {
			if i := SrelaxboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := SrxcountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		} else {
			if i := SleaderboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := ScountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		}
		resp.Users = append(resp.Users, u)
	}
	return resp
}

func SleaderboardPosition(r *redis.Client, mode string, user int) *int {
	return S_position(r, "ripple:leaderboard:"+mode, user)
}

func ScountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return S_position(r, "ripple:leaderboard:"+mode+":"+strings.ToLower(country), user)
}

func SrelaxboardPosition(r *redis.Client, mode string, user int) *int {
	return _position(r, "ripple:relaxboard:"+mode, user)
}

func SrxcountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return _position(r, "ripple:relaxboard:"+mode+":"+strings.ToLower(country), user)
}

func S_position(r *redis.Client, key string, user int) *int {
	res := r.ZRevRank(key, strconv.Itoa(user))
	if res.Err() == redis.Nil {
		return nil
	}
	x := int(res.Val()) + 1
	return &x
}
