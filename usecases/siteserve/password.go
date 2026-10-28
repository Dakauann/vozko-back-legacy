package siteserve_usecase

import (
	"net/http"

	"vozko/domain/sitepublish"
)

func (s *Service) passwordGate(v visit) (Outcome, bool) {
	if v.Path == PasswordPath && v.Method == http.MethodPost {
		return s.unlock(v), true
	}
	if sitepublish.ValidPassToken(s.cfg.CookieSecret, v.host, v.settings, v.Cookie(PassCookie), s.now()) {
		return Outcome{}, false
	}
	next := v.Path
	if v.RawQuery != "" {
		next += "?" + v.RawQuery
	}
	return Outcome{Kind: KindPasswordPrompt, Prompt: Prompt{Next: sitepublish.SafeNext(next)}}, true
}

func (s *Service) unlock(v visit) Outcome {
	if !allowed(s.passwords, v.ClientIP+"|"+v.host) {
		return Outcome{Kind: KindTooManyRequests}
	}
	next := sitepublish.SafeNext(v.Next)
	if !sitepublish.VerifyPagePassword(v.Password, v.settings.PasswordHash) {
		return Outcome{Kind: KindPasswordPrompt, Prompt: Prompt{Next: next, Failed: true}}
	}
	token := sitepublish.PassToken(s.cfg.CookieSecret, v.host, v.settings, s.now())
	return Outcome{
		Kind: KindRedirect, Location: next, Status: http.StatusSeeOther,
		Cookies: []Cookie{{Name: PassCookie, Value: token, Path: "/", MaxAge: sitepublish.PassTokenLifetime, HTTPOnly: true}},
	}
}
