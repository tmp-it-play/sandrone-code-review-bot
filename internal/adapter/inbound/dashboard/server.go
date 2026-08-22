package dashboard

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type Server struct {
	deps      Dependencies
	templates *template.Template
}

func NewServer(deps Dependencies) (*Server, error) {
	pattern := filepath.Join(deps.TemplateDir, "*.html")
	templates, err := template.ParseGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("대시보드 템플릿을 읽지 못했다: %w", err)
	}
	return &Server{deps: deps, templates: templates}, nil
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard/login", s.loginForm)
	mux.HandleFunc("POST /dashboard/login", s.login)
	mux.HandleFunc("POST /dashboard/logout", s.logout)
	mux.Handle("GET /dashboard/static/", http.StripPrefix("/dashboard/static/", http.FileServer(http.Dir(s.deps.StaticDir))))
	mux.Handle("GET /dashboard", s.guard(http.HandlerFunc(s.overview)))
	mux.Handle("GET /dashboard/", s.guard(http.HandlerFunc(s.overview)))
	mux.Handle("GET /dashboard/reviews", s.guard(http.HandlerFunc(s.reviews)))
	mux.Handle("GET /dashboard/reviews/{id}", s.guard(http.HandlerFunc(s.reviewDetail)))
	mux.Handle("POST /dashboard/reviews/{id}/rerun", s.guard(http.HandlerFunc(s.rerun)))
	mux.Handle("GET /dashboard/providers", s.guard(http.HandlerFunc(s.providers)))
	mux.Handle("GET /dashboard/queue", s.guard(http.HandlerFunc(s.queue)))
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !s.deps.Sessions.Valid(request) {
			http.Redirect(writer, request, s.deps.BasePath+"/dashboard/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) loginForm(writer http.ResponseWriter, request *http.Request) {
	if s.deps.Sessions.Valid(request) {
		http.Redirect(writer, request, s.deps.BasePath+"/dashboard", http.StatusSeeOther)
		return
	}
	s.render(writer, "login.html", PageData{Title: "로그인"})
}

func (s *Server) login(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		s.render(writer, "login.html", PageData{Title: "로그인", Error: "입력을 읽지 못했다."})
		return
	}
	username := request.PostFormValue("username")
	password := request.PostFormValue("password")
	if !s.deps.Credentials.Matches(username, password) {
		writer.WriteHeader(http.StatusUnauthorized)
		s.render(writer, "login.html", PageData{Title: "로그인", Error: "아이디 또는 비밀번호가 올바르지 않다."})
		return
	}
	s.deps.Sessions.Issue(writer, username)
	http.Redirect(writer, request, s.deps.BasePath+"/dashboard", http.StatusSeeOther)
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	s.deps.Sessions.Clear(writer)
	http.Redirect(writer, request, s.deps.BasePath+"/dashboard/login", http.StatusSeeOther)
}

func (s *Server) overview(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	data := PageData{Title: "개요", Active: "overview"}
	repositories, err := s.deps.Installations.Repositories(ctx)
	if err != nil {
		s.deps.Logger.Warn("저장소 목록을 읽지 못했다", "error", err)
	}
	for _, repository := range repositories {
		data.Repositories = append(data.Repositories, RepositoryView{
			FullName:       repository.FullName(),
			InstallationID: repository.InstallationID,
			Private:        repository.Private,
		})
	}
	data.Reviews = s.recentReviews(ctx, 10)
	data.Providers = s.providerViews(ctx)
	data.Queue = s.queueView()
	data.Commands = s.recentCommands(ctx, 10)
	s.render(writer, "overview.html", data)
}

func (s *Server) reviews(writer http.ResponseWriter, request *http.Request) {
	data := PageData{Title: "리뷰 이력", Active: "reviews"}
	data.Reviews = s.recentReviews(request.Context(), 100)
	s.render(writer, "reviews.html", data)
}

func (s *Server) reviewDetail(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseUint(request.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	record, err := s.deps.Reviews.ByID(request.Context(), id)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	findings, err := s.deps.Findings.ByReview(request.Context(), id)
	if err != nil {
		s.deps.Logger.Warn("리뷰 지적을 읽지 못했다", "review", id, "error", err)
	}
	data := PageData{Title: "리뷰 상세", Active: "reviews", Review: toReviewView(record)}
	for _, finding := range findings {
		data.Findings = append(data.Findings, FindingView{
			Path:      finding.File,
			Line:      finding.Line,
			Severity:  string(finding.Severity),
			Title:     finding.Title,
			Body:      finding.Body,
			Placement: string(finding.Placement),
		})
	}
	s.render(writer, "review.html", data)
}

func (s *Server) rerun(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseUint(request.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	record, err := s.deps.Reviews.ByID(request.Context(), id)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	target := pullrequest.Target{
		Owner:      record.Owner,
		Repository: record.Repository,
		Number:     record.Number,
		HeadSHA:    record.HeadSHA,
	}
	installationID, found := s.installationFor(request.Context(), record.Owner, record.Repository)
	if !found {
		http.Error(writer, "설치 정보를 찾지 못했다", http.StatusBadRequest)
		return
	}
	target.InstallationID = installationID
	err = s.deps.Queue.EnqueueReview(request.Context(), job.ReviewJob{
		Target:  target,
		Trigger: review.TriggerDashboardRerun,
	})
	if err != nil {
		http.Error(writer, "재실행을 요청하지 못했다", http.StatusInternalServerError)
		return
	}
	http.Redirect(writer, request, s.deps.BasePath+"/dashboard/reviews", http.StatusSeeOther)
}

func (s *Server) providers(writer http.ResponseWriter, request *http.Request) {
	data := PageData{Title: "프로바이더", Active: "providers"}
	data.Providers = s.providerViews(request.Context())
	s.render(writer, "providers.html", data)
}

func (s *Server) queue(writer http.ResponseWriter, request *http.Request) {
	data := PageData{Title: "큐", Active: "queue", Queue: s.queueView()}
	s.render(writer, "queue.html", data)
}

func (s *Server) installationFor(ctx context.Context, owner string, repository string) (int64, bool) {
	repositories, err := s.deps.Installations.Repositories(ctx)
	if err != nil {
		return 0, false
	}
	for _, entry := range repositories {
		if entry.Owner == owner && entry.Name == repository {
			return entry.InstallationID, true
		}
	}
	return 0, false
}

func (s *Server) recentReviews(ctx context.Context, limit int) []ReviewView {
	records, err := s.deps.Reviews.Recent(ctx, limit)
	if err != nil {
		s.deps.Logger.Warn("리뷰 이력을 읽지 못했다", "error", err)
		return nil
	}
	views := make([]ReviewView, 0, len(records))
	for _, record := range records {
		views = append(views, toReviewView(record))
	}
	return views
}

func (s *Server) recentCommands(ctx context.Context, limit int) []CommandView {
	invocations, err := s.deps.Commands.Recent(ctx, limit)
	if err != nil {
		s.deps.Logger.Warn("명령 이력을 읽지 못했다", "error", err)
		return nil
	}
	views := make([]CommandView, 0, len(invocations))
	for _, invocation := range invocations {
		views = append(views, CommandView{
			Repository: invocation.Owner + "/" + invocation.Repository,
			Number:     invocation.Number,
			Invoker:    invocation.Invoker,
			Kind:       string(invocation.Kind),
			Allowed:    invocation.Allowed,
			OccurredAt: formatTime(invocation.OccurredAt),
		})
	}
	return views
}

func (s *Server) providerViews(ctx context.Context) []ProviderView {
	snapshots, err := s.deps.Usage.Snapshot(ctx)
	if err != nil {
		s.deps.Logger.Warn("프로바이더 사용량을 읽지 못했다", "error", err)
	}
	byName := map[string]ProviderView{}
	for _, snapshot := range snapshots {
		byName[snapshot.Provider] = ProviderView{
			Name:         snapshot.Provider,
			Succeeded:    snapshot.Succeeded,
			Failed:       snapshot.Failed,
			QuotaBlocked: snapshot.QuotaBlocked,
			LastUsedAt:   formatTime(snapshot.LastUsedAt),
		}
	}
	views := make([]ProviderView, 0, len(s.deps.ProviderOrder))
	for index, name := range s.deps.ProviderOrder {
		view, ok := byName[name]
		if !ok {
			view = ProviderView{Name: name}
		}
		view.Order = index + 1
		if endsAt, active, err := s.deps.Cooldown.EndsAt(ctx, name); err == nil && active {
			view.CooldownEnds = formatTime(endsAt)
		}
		views = append(views, view)
	}
	return views
}

func (s *Server) queueView() QueueView {
	if s.deps.Inspector == nil {
		return QueueView{}
	}
	queues, err := s.deps.Inspector.Queues()
	if err != nil {
		s.deps.Logger.Warn("큐 목록을 읽지 못했다", "error", err)
		return QueueView{}
	}
	view := QueueView{Available: true}
	for _, name := range queues {
		info, infoErr := s.deps.Inspector.GetQueueInfo(name)
		if infoErr != nil {
			continue
		}
		view.Pending += info.Pending
		view.Active += info.Active
		view.Retry += info.Retry
		view.Archived += info.Archived
		view.Completed += info.Completed
	}
	return view
}

func (s *Server) render(writer http.ResponseWriter, name string, data PageData) {
	data.Base = s.deps.BasePath
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(writer, name, data); err != nil {
		s.deps.Logger.Error("템플릿을 렌더링하지 못했다", "template", name, "error", err)
	}
}

func toReviewView(record review.Record) ReviewView {
	return ReviewView{
		ID:         record.ID,
		Repository: record.Owner + "/" + record.Repository,
		Number:     record.Number,
		Trigger:    string(record.Trigger),
		Outcome:    string(record.Outcome),
		Provider:   record.Provider,
		Model:      record.Model,
		Inline:     record.InlineCount,
		Fallback:   record.FallbackCount,
		Duration:   record.Duration().Round(time.Second).String(),
		StartedAt:  formatTime(record.StartedAt),
		Detail:     record.Detail,
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
