package shared

import (
	"Elitebabes.com/elite_model"
	"fmt"
	"github.com/fatih/color"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/mitchellh/go-ps"
	"os"
)

const ReferralPercent = 0.10

func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		color.Red("No .env file found")
	}
}

func ConnectToDb() *sqlx.DB {
	db, err := sqlx.Connect("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s "+
		"sslmode=disable port=%s", os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASS"),
		os.Getenv("DB_NAME"), os.Getenv("DB_PORT")))
	if err != nil {
		panic(err)
	}
	return db
}

func SingleProcess(name string) {
	processList, err := ps.Processes()
	if err != nil {
		panic(err)
	}

	var count = 0
	for x := range processList {
		var process ps.Process
		process = processList[x]
		if process.Executable() == name {
			count++
		}
	}

	if count > 1 {
		os.Exit(0)
	}
}

func PluralPostfix(count int) string {
	switch {
	case 5 <= count && count <= 20:
		return "ов"
	case count%10 == 1:
		return ""
	case count%10 == 0:
		return "ов"
	case count%10 <= 4:
		return "а"
	default:
		return "ов"
	}
}

func AddBonus(db *sqlx.DB, userId int, bonus float32, level int) {
	_, _ = db.Exec("INSERT INTO bonuses (from_id, bonus) "+
		"VALUES ($1, $2) "+
		"ON CONFLICT (from_id) DO UPDATE "+
		"SET bonus = bonuses.bonus + excluded.bonus",
		userId, bonus)

	if level != 1 {
		var referral = elite_model.Referral{}
		var err = db.Get(&referral, "SELECT parent_id from referrals where user_id = $1", userId)
		if err == nil {
			AddBonus(db, referral.ParentId, bonus*ReferralPercent, level-1)
		}
	}
}
