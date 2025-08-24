/*

GoGet GoFmt GoBuildNull

*/

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/shoce/tg"
)

const (
	NL = "\n"
)

type TgMoonConfig struct {
	YssUrl string `yaml:"-"`

	DEBUG bool `yaml:"DEBUG"`

	Interval time.Duration `yaml:"Interval"`

	TgApiUrlBase string `yaml:"TgApiUrlBase"` // = "https://api.telegram.org"

	TgToken  string `yaml:"TgToken"`
	TgChatId string `yaml:"TgChatId"`

	PostingStartHour int `yaml:"PostingStartHour"`

	MoonPhaseLast string `yaml:"MoonPhaseLast"`
}

var (
	Config TgMoonConfig

	TZIST = time.FixedZone("IST", 330*60)

	Ctx context.Context

	HttpClient = &http.Client{}
)

func init() {
	Ctx = context.TODO()

	if s := os.Getenv("YssUrl"); s != "" {
		Config.YssUrl = s
	}
	if Config.YssUrl == "" {
		log("ERROR YssUrl empty")
		os.Exit(1)
	}

	if err := Config.Get(); err != nil {
		log("ERROR Config.Get %v", err)
		os.Exit(1)
	}

	if Config.DEBUG {
		log("DEBUG <true>")
	}

	log("Interval <%v>", Config.Interval)
	if Config.Interval == 0 {
		log("ERROR Interval <0>")
		os.Exit(1)
	}

	if Config.TgToken == "" {
		log("ERROR TgToken empty")
		os.Exit(1)
	}

	tg.ApiToken = Config.TgToken

	if Config.TgChatId == "" {
		log("ERROR TgChatId empty")
		os.Exit(1)
	}

	if Config.PostingStartHour < 0 || Config.PostingStartHour > 23 {
		log("ERROR invalid PostingStartHour <%d> must be between <0> and <23>", Config.PostingStartHour)
		os.Exit(1)
	}
}

func main() {
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	go func(sigterm chan os.Signal) {
		<-sigterm
		tglog("%s sigterm", os.Args[0])
		os.Exit(1)
	}(sigterm)

	for {
		t0 := time.Now()

		if err := PostMoonPhase(); err != nil {
			tglog("ERROR PostMoonPhase %v", err)
		}

		if dur := time.Now().Sub(t0); dur < Config.Interval {
			time.Sleep(Config.Interval - dur)
		}
	}
}

func PostMoonPhase() error {
	if time.Now().UTC().Hour() < Config.PostingStartHour {
		return nil
	}

	yearmonthday := time.Now().UTC().Format("2006/Jan/2")
	if yearmonthday == Config.MoonPhaseLast {
		return nil
	}

	if moonphase := MoonPhase(); moonphase != "" {
		if _, tgerr := tg.SendMessage(tg.SendMessageRequest{
			ChatId: Config.TgChatId,
			Text:   tg.Esc(moonphase),

			LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
		}); tgerr != nil {
			return tgerr
		}
	}

	Config.MoonPhaseLast = yearmonthday
	if err := Config.Put(); err != nil {
		return fmt.Errorf("ERROR Config.Put %v", err)
	}

	return nil
}

func MoonPhaseCalendar() string {
	nmfm := []string{"○", "●"}
	const MoonCycleDur time.Duration = 2551443 * time.Second
	var NewMoon time.Time = time.Date(2020, time.December, 14, 16, 16, 0, 0, time.UTC)
	var sinceNM time.Duration = time.Since(NewMoon) % MoonCycleDur
	var lastNM time.Time = time.Now().UTC().Add(-sinceNM)
	var msg, year, month string
	var mo time.Time = lastNM
	for i := 0; mo.Before(lastNM.Add(time.Hour * 24 * 7 * 54)); i++ {
		if mo.Format("2006") != year {
			year = mo.Format("2006")
			msg += NL + NL + fmt.Sprintf("Year %s", year) + NL
		}
		if mo.Format("Jan") != month {
			month = mo.Format("Jan")
			msg += NL + fmt.Sprintf("%s ", month)
		}
		msg += fmt.Sprintf(
			"%s:%s ",
			mo.Add(-4*time.Hour).Format("Mon/2"),
			nmfm[i%2],
		)
		mo = mo.Add(MoonCycleDur / 2)
	}
	return msg
}

func MoonPhase() string {
	// https://www.timeanddate.com/moon/phases/timezone/utc
	// https://pkg.go.dev/time
	//const MoonCycleDur time.Duration = 2551443 * time.Second
	//var NewMoon1 time.Time = time.Date(2000, time.January, 6, 18, 13, 0, 0, time.UTC)
	var NewMoon1 time.Time = time.Date(2020, time.December, 14, 16, 16, 0, 0, time.UTC)
	var NewMoon2 time.Time = time.Date(2025, time.June, 25, 10, 31, 0, 0, time.UTC)
	var MoonCycleDur time.Duration = NewMoon2.Sub(NewMoon1) / 56
	var NewMoon time.Time = NewMoon2
	var tnow time.Time = time.Now().UTC()

	var sinceNew time.Duration = tnow.Sub(NewMoon) % MoonCycleDur
	if sinceNew < 24*time.Hour {
		return fmt.Sprintf(
			"New Moon was at %s.",
			tnow.Add(-sinceNew).In(TZIST).Format("15:04 Monday, January 2"),
		)
	}
	if tillNew := MoonCycleDur - sinceNew; tillNew < 24*time.Hour {
		return fmt.Sprintf(
			"New Moon at %s; next Full Moon on %s.",
			tnow.Add(tillNew).In(TZIST).Format("15:04 Monday, January 2"),
			tnow.Add(MoonCycleDur/2).In(TZIST).Format("Monday, January 2"),
		)
	}

	var sinceFull time.Duration = sinceNew + MoonCycleDur/2
	if sinceFull < 24*time.Hour {
		return fmt.Sprintf(
			"Full Moon was at %s.",
			tnow.Add(-sinceFull).In(TZIST).Format("15:04 Monday, January 2"),
		)
	}
	if tillFull := MoonCycleDur/2 - sinceNew; tillFull >= 0 && tillFull < 24*time.Hour {
		return fmt.Sprintf(
			"Full Moon at %s; next New Moon on %s.",
			tnow.Add(tillFull).In(TZIST).Format("15:04 Monday, January 2"),
			tnow.Add(MoonCycleDur/2).In(TZIST).Format("Monday, January 2"),
		)
	}

	return ""
}

func ts() string {
	tnow := time.Now().In(TZIST)
	return fmt.Sprintf(
		"%d%02d%02d:%02d%02d+",
		tnow.Year()%1000, tnow.Month(), tnow.Day(),
		tnow.Hour(), tnow.Minute(),
	)
}

func log(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+msg+NL, args...)
}

func tglog(msg string, args ...interface{}) (err error) {
	log(msg, args...)
	_, err = tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.TgChatId,
		Text:   tg.Esc(msg, args...),

		DisableNotification: true,
		LinkPreviewOptions:  tg.LinkPreviewOptions{IsDisabled: true},
	})
	return err
}

func (config *TgMoonConfig) Get() error {
	req, err := http.NewRequest(http.MethodGet, config.YssUrl, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("yss response status %s", resp.Status)
	}

	rbb, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(rbb, config); err != nil {
		return err
	}

	if Config.DEBUG {
		//log("DEBUG Config.Get %+v", config)
	}

	return nil
}

func (config *TgMoonConfig) Put() error {
	if config.DEBUG {
		//log("DEBUG Config.Put %s %+v", config.YssUrl, config)
	}

	rbb, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, config.YssUrl, bytes.NewBuffer(rbb))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("yss response status %s", resp.Status)
	}

	return nil
}
