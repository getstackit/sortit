package matheval

import (
	"hash/fnv"
	"math"
	"strconv"
)

// GenerateCorpus deterministically builds the fixture corpus that is checked
// into testdata/corpus.json. Embeddings are synthesized, not learned: each
// tag direction is a hash-derived unit vector correlated with its domain
// (billing≈payments, auth≈login, ...), and each issue/query embedding is the
// relevance-weighted sum of its tag directions plus hash noise. The noise
// share varies per issue so the corpus exhibits a realistic spread of
// per-issue R² — some issues are almost fully explained by their tags, some
// are mostly residual, and untagged notes are pure residual.
//
// Every synthesized vector additionally shares a corpus-wide common
// direction (anisotropy), mirroring real text-embedding models, where two
// unrelated same-corpus texts still cosine at ~0.5–0.7 (whitepaper §2.0).
// Without it the fixtures cannot exercise the corpus-mean centering that the
// runtime applies, and any constant tuned here would be tuned against an
// unrealistically isotropic geometry.
//
// Regenerate testdata/corpus.json with:
//
//	go test ./internal/matheval -run TestCorpusMatchesGenerator -update
func GenerateCorpus() Corpus {
	tagVecs := make(map[string][]float64, len(tagSpecs))
	corpus := Corpus{Dim: EmbeddingDim}

	for _, spec := range tagSpecs {
		vec := combine(
			scaled(anisotropyVec(), anisotropyScale),
			scaled(noiseVec("domain:"+spec.domain), 0.9),
			scaled(noiseVec("tag:"+spec.name), 0.45),
		)
		normalize(vec)
		roundVec(vec)
		tagVecs[spec.name] = vec
		corpus.Tags = append(corpus.Tags, CorpusTag{
			Name:        spec.name,
			Description: spec.description,
			Specificity: spec.specificity,
			Embedding:   vec,
		})
	}

	for _, spec := range issueSpecs {
		emb := synthesizeEmbedding("issue:"+spec.id, spec.tags, spec.noise, tagVecs)
		issue := CorpusIssue{
			ID:        spec.id,
			Raw:       spec.raw,
			TagScores: spec.tags,
			Embedding: emb,
		}
		if spec.velocity > 0 {
			velocity := spec.velocity
			issue.Velocity = &velocity
		}
		corpus.Issues = append(corpus.Issues, issue)
	}

	for _, spec := range querySpecs {
		corpus.Queries = append(corpus.Queries, CorpusQuery{
			ID:        spec.id,
			Raw:       spec.raw,
			Tags:      spec.tags,
			Embedding: synthesizeEmbedding("query:"+spec.id, spec.tags, spec.noise, tagVecs),
		})
	}

	return corpus
}

// anisotropyScale sets the magnitude of the shared common direction mixed
// into every synthesized vector. The common direction enters issue and
// query embeddings twice — once directly and once through their tag
// directions, which also carry it — so the effective shared component is
// larger than this constant suggests. 0.55 lands the mean raw pairwise
// cosine across the corpus at ~0.58, inside the ~0.5–0.7 band observed on
// real OpenAI embeddings over a homogeneous corpus; corpus-mean centering
// removes it almost entirely (TestGeneratedCorpusIsAnisotropic pins both).
const anisotropyScale = 0.55

// anisotropyVec is the corpus-wide common direction. Tags, issues, and
// queries all share the same one — in reality all text goes through the
// same embedding model, whose common component does not depend on whether
// the text is a tag description or an issue.
func anisotropyVec() []float64 {
	return noiseVec("corpus:anisotropy")
}

// synthesizeEmbedding builds a unit embedding from tag directions plus seeded
// noise. With no tags the content part is pure noise (an "off-taxonomy"
// text). The shared anisotropic direction is added on top either way: even
// off-taxonomy text comes out of the same embedding model.
func synthesizeEmbedding(seed string, tags []TagScore, noise float64, tagVecs map[string][]float64) []float64 {
	vec := scaled(anisotropyVec(), anisotropyScale)
	for _, ts := range tags {
		vec = combine(vec, scaled(tagVecs[ts.Tag], ts.Relevance))
	}
	vec = combine(vec, scaled(noiseVec(seed), noise))
	normalize(vec)
	roundVec(vec)
	return vec
}

// noiseVec returns a deterministic pseudo-random unit vector for a seed,
// derived from FNV-1a hashes so it is stable across platforms and Go
// versions (no math/rand dependence).
func noiseVec(seed string) []float64 {
	vec := make([]float64, EmbeddingDim)
	for d := range vec {
		h := fnv.New64a()
		_, _ = h.Write([]byte(seed))
		_, _ = h.Write([]byte("#"))
		_, _ = h.Write([]byte(strconv.Itoa(d)))
		// Map the hash to [-1, 1).
		vec[d] = float64(int64(h.Sum64()%2_000_001))/1_000_000 - 1
	}
	normalize(vec)
	return vec
}

func combine(vecs ...[]float64) []float64 {
	out := make([]float64, len(vecs[0]))
	for _, vec := range vecs {
		for i := range vec {
			out[i] += vec[i]
		}
	}
	return out
}

func scaled(v []float64, s float64) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] * s
	}
	return out
}

func normalize(v []float64) {
	var mag float64
	for _, x := range v {
		mag += x * x
	}
	if mag == 0 {
		return
	}
	mag = math.Sqrt(mag)
	for i := range v {
		v[i] /= mag
	}
}

// roundVec rounds to 6 decimals to keep the checked-in JSON readable. The
// ~1e-6 unit-norm error is absorbed by the runtime, which re-normalizes
// stored embeddings.
func roundVec(v []float64) {
	for i := range v {
		v[i] = math.Round(v[i]*1e6) / 1e6
	}
}

type tagSpec struct {
	name        string
	domain      string
	specificity float64
	description string
}

// tagSpecs defines the fixture taxonomy. Domains induce correlation between
// related tags; specificity below scoring.GenericTagThreshold (0.5) marks a
// tag as a generic bucket.
var tagSpecs = []tagSpec{
	{"billing", "money", 0.75, "Billing, invoices, charges, and refunds"},
	{"payments", "money", 0.80, "Payment processing and card transactions"},
	{"auth", "identity", 0.70, "Authentication and session handling"},
	{"login", "identity", 0.75, "Login flows and credentials"},
	{"export", "documents", 0.70, "Exporting data and documents"},
	{"pdf", "documents", 0.85, "PDF generation and rendering"},
	{"search", "discovery", 0.65, "Search indexing and ranking"},
	{"performance", "performance", 0.50, "Latency, throughput, and resource usage"},
	{"mobile", "mobile", 0.55, "Mobile apps in general"},
	{"ios", "mobile", 0.80, "iOS-specific behavior"},
	{"notifications", "communications", 0.70, "Push and in-app notifications"},
	{"email", "communications", 0.75, "Email delivery and templates"},
	{"database", "data", 0.60, "Database queries and storage"},
	{"migration", "data", 0.80, "Schema migrations"},
	{"ui", "general", 0.30, "General user interface"},
	{"backend", "general", 0.25, "General backend/server work"},
}

type issueSpec struct {
	id       string
	raw      string
	tags     []TagScore
	noise    float64 // residual share mixed into the embedding
	velocity float64 // optional lifecycle velocity (0 = unset)
}

var issueSpecs = []issueSpec{
	// Money: billing and payments.
	{id: "bill-01", raw: "Customer charged twice for the same invoice after retrying a failed card payment", noise: 0.35, velocity: 0.7,
		tags: []TagScore{{"billing", 0.9}, {"payments", 0.85}, {"backend", 0.3}}},
	{id: "bill-02", raw: "Refund issued in Stripe but invoice still shows as unpaid in the billing dashboard", noise: 0.4,
		tags: []TagScore{{"billing", 0.9}, {"payments", 0.7}, {"ui", 0.25}}},
	{id: "bill-03", raw: "Proration math wrong when upgrading mid-cycle, invoice total off by one day", noise: 0.3,
		tags: []TagScore{{"billing", 0.95}, {"backend", 0.2}}},
	{id: "bill-04", raw: "VAT not applied to invoices for EU customers on annual plans", noise: 0.45,
		tags: []TagScore{{"billing", 0.85}}},
	{id: "pay-01", raw: "3DS challenge loop: card payment never completes on checkout", noise: 0.35,
		tags: []TagScore{{"payments", 0.95}, {"billing", 0.4}}},
	{id: "pay-02", raw: "Webhook from payment provider dropped, subscription stuck in past_due", noise: 0.5,
		tags: []TagScore{{"payments", 0.8}, {"backend", 0.5}, {"billing", 0.5}}},
	{id: "pay-03", raw: "Apple Pay button missing on mobile checkout", noise: 0.55,
		tags: []TagScore{{"payments", 0.8}, {"mobile", 0.6}, {"ui", 0.4}}},

	// Identity: auth and login.
	{id: "auth-01", raw: "Session expires mid-form and silently discards user input", noise: 0.4,
		tags: []TagScore{{"auth", 0.9}, {"ui", 0.4}}},
	{id: "auth-02", raw: "OAuth login with Google returns 500 when email contains a plus sign", noise: 0.35,
		tags: []TagScore{{"auth", 0.85}, {"login", 0.9}, {"backend", 0.3}}},
	{id: "auth-03", raw: "Password reset link valid for 24 hours but the email says it expires in one hour", noise: 0.5,
		tags: []TagScore{{"auth", 0.7}, {"login", 0.6}, {"email", 0.6}}},
	{id: "auth-04", raw: "JWT refresh race signs the user out when multiple tabs renew in parallel", noise: 0.4,
		tags: []TagScore{{"auth", 0.9}, {"backend", 0.45}}},
	{id: "login-01", raw: "Login page keeps spinning forever in Safari private browsing mode", noise: 0.45,
		tags: []TagScore{{"login", 0.9}, {"auth", 0.6}, {"ui", 0.35}}},
	{id: "login-02", raw: "Two-factor code rejected when entered with a trailing space", noise: 0.3,
		tags: []TagScore{{"login", 0.85}, {"auth", 0.8}}},

	// Documents: export and PDF.
	{id: "exp-01", raw: "PDF export renders blank pages for documents with embedded charts", noise: 0.3,
		tags: []TagScore{{"export", 0.9}, {"pdf", 0.95}}},
	{id: "exp-02", raw: "CSV export drops rows when a cell contains a newline", noise: 0.4,
		tags: []TagScore{{"export", 0.9}, {"backend", 0.3}}},
	{id: "exp-03", raw: "Export as PDF times out for projects with more than five hundred issues", noise: 0.35,
		tags: []TagScore{{"export", 0.85}, {"pdf", 0.8}, {"performance", 0.6}}},
	{id: "exp-04", raw: "Scheduled export email arrives with a corrupted attachment", noise: 0.5,
		tags: []TagScore{{"export", 0.8}, {"email", 0.65}}},
	{id: "pdf-01", raw: "PDF generation uses the wrong font and clips long issue titles", noise: 0.45,
		tags: []TagScore{{"pdf", 0.9}, {"export", 0.6}, {"ui", 0.4}}},
	{id: "pdf-02", raw: "Right-to-left text comes out reversed in generated PDFs", noise: 0.4,
		tags: []TagScore{{"pdf", 0.9}, {"export", 0.55}}},

	// Discovery and performance.
	{id: "srch-01", raw: "Search returns stale results for ten minutes after an issue is renamed", noise: 0.4, velocity: 0.8,
		tags: []TagScore{{"search", 0.9}, {"backend", 0.4}}},
	{id: "srch-02", raw: "Search ranking puts exact title matches below fuzzy matches", noise: 0.3,
		tags: []TagScore{{"search", 0.95}}},
	{id: "srch-03", raw: "Typing in the search box freezes the page on large workspaces", noise: 0.4,
		tags: []TagScore{{"search", 0.8}, {"performance", 0.8}, {"ui", 0.4}}},
	{id: "perf-01", raw: "Dashboard takes twelve seconds to load for organizations with fifty thousand issues", noise: 0.45,
		tags: []TagScore{{"performance", 0.9}, {"database", 0.6}, {"backend", 0.4}}},
	{id: "perf-02", raw: "Memory leak in the background indexer after a week of uptime", noise: 0.5,
		tags: []TagScore{{"performance", 0.85}, {"search", 0.5}, {"backend", 0.5}}},
	{id: "perf-03", raw: "N+1 queries when listing issues together with their tags", noise: 0.35,
		tags: []TagScore{{"performance", 0.8}, {"database", 0.85}, {"backend", 0.4}}},

	// Mobile.
	{id: "mob-01", raw: "App crashes on launch after upgrading to the iOS 18 beta", noise: 0.3, velocity: 0.6,
		tags: []TagScore{{"mobile", 0.85}, {"ios", 0.95}}},
	{id: "mob-02", raw: "Pull-to-refresh duplicates list items on the issues screen", noise: 0.45,
		tags: []TagScore{{"mobile", 0.85}, {"ui", 0.5}}},
	{id: "mob-03", raw: "Offline edits lost when the app is backgrounded for an hour", noise: 0.5,
		tags: []TagScore{{"mobile", 0.9}, {"backend", 0.3}}},
	{id: "mob-04", raw: "Android back gesture closes the app instead of dismissing the modal", noise: 0.55,
		tags: []TagScore{{"mobile", 0.8}, {"ui", 0.5}}},
	{id: "ios-01", raw: "Tapping a push notification opens the wrong issue on iOS", noise: 0.4,
		tags: []TagScore{{"ios", 0.85}, {"mobile", 0.7}, {"notifications", 0.8}}},
	{id: "ios-02", raw: "Keyboard covers the comment box on iPhone SE", noise: 0.45,
		tags: []TagScore{{"ios", 0.8}, {"mobile", 0.7}, {"ui", 0.6}}},

	// Communications.
	{id: "notif-01", raw: "Duplicate push notifications sent for a single mention", noise: 0.4,
		tags: []TagScore{{"notifications", 0.9}, {"backend", 0.35}}},
	{id: "notif-02", raw: "Notification preferences ignored after two accounts are merged", noise: 0.5,
		tags: []TagScore{{"notifications", 0.85}, {"auth", 0.4}}},
	{id: "notif-03", raw: "In-app notification badge count never clears", noise: 0.45,
		tags: []TagScore{{"notifications", 0.8}, {"ui", 0.5}}},
	{id: "email-01", raw: "Digest email renders broken in Outlook dark mode", noise: 0.45,
		tags: []TagScore{{"email", 0.9}, {"ui", 0.5}}},
	{id: "email-02", raw: "Bounced emails are never retried and no alert is raised", noise: 0.5,
		tags: []TagScore{{"email", 0.85}, {"backend", 0.45}}},
	{id: "email-03", raw: "Invitation emails land in spam for corporate domains", noise: 0.4,
		tags: []TagScore{{"email", 0.85}}},

	// Data.
	{id: "db-01", raw: "Deadlock between the issue update and tag recount transactions", noise: 0.35,
		tags: []TagScore{{"database", 0.9}, {"backend", 0.5}}},
	{id: "db-02", raw: "Migration 0042 fails on Postgres 14 due to a concurrent index build", noise: 0.3,
		tags: []TagScore{{"migration", 0.95}, {"database", 0.8}}},
	{id: "db-03", raw: "Read replica lag causes stale issue counts on the dashboard", noise: 0.5,
		tags: []TagScore{{"database", 0.85}, {"performance", 0.55}, {"backend", 0.4}}},
	{id: "db-04", raw: "Nightly vacuum window overlaps with the backup job and stalls writes", noise: 0.55,
		tags: []TagScore{{"database", 0.8}, {"performance", 0.5}}},
	{id: "mig-01", raw: "Rolling back a failed migration leaves orphaned columns behind", noise: 0.4,
		tags: []TagScore{{"migration", 0.9}, {"database", 0.75}}},

	// Cross-cutting, generic, and off-taxonomy notes.
	{id: "gen-01", raw: "Improve the onboarding flow copy and empty states", noise: 0.8,
		tags: []TagScore{{"ui", 0.6}}},
	{id: "gen-02", raw: "Refactor service layer error handling into a shared helper", noise: 0.85,
		tags: []TagScore{{"backend", 0.7}}},
	{id: "gen-03", raw: "Dark mode toggle does not persist across sessions", noise: 0.6,
		tags: []TagScore{{"ui", 0.8}}},
	{id: "misc-01", raw: "Weekly sync notes: discussed roadmap priorities and hiring", noise: 1},
	{id: "misc-02", raw: "Spike: evaluate feasibility of a plugin marketplace", noise: 1},
	{id: "misc-03", raw: "Investigate flaky CI job on main", noise: 1},
}

type querySpec struct {
	id    string
	raw   string
	tags  []TagScore
	noise float64
}

// querySpecs are the search queries the judgment set labels. Queries are
// slightly cleaner than issues (lower noise) since they are short and
// topical; a few are bare tag names to exercise the tag-correlation and
// generic-tag search paths.
var querySpecs = []querySpec{
	{id: "q-double-charge", raw: "customer double charged for invoice after card retry", noise: 0.35,
		tags: []TagScore{{"billing", 0.9}, {"payments", 0.8}}},
	{id: "q-refund-unpaid", raw: "refund processed but invoice still unpaid", noise: 0.35,
		tags: []TagScore{{"billing", 0.85}, {"payments", 0.6}}},
	{id: "q-checkout-3ds", raw: "checkout payment stuck in 3DS verification", noise: 0.35,
		tags: []TagScore{{"payments", 0.9}}},
	{id: "q-tag-billing", raw: "billing", noise: 0.25,
		tags: []TagScore{{"billing", 0.9}}},
	{id: "q-past-due", raw: "subscription stuck past due after webhook failure", noise: 0.4,
		tags: []TagScore{{"payments", 0.7}, {"backend", 0.5}, {"billing", 0.5}}},
	{id: "q-google-500", raw: "google login returns 500 error", noise: 0.35,
		tags: []TagScore{{"login", 0.9}, {"auth", 0.8}}},
	{id: "q-parallel-logout", raw: "randomly logged out with multiple tabs open", noise: 0.4,
		tags: []TagScore{{"auth", 0.85}}},
	{id: "q-reset-expiry", raw: "password reset email shows wrong expiry time", noise: 0.4,
		tags: []TagScore{{"auth", 0.6}, {"login", 0.6}, {"email", 0.6}}},
	{id: "q-2fa-rejected", raw: "two factor code not accepted", noise: 0.35,
		tags: []TagScore{{"login", 0.85}, {"auth", 0.7}}},
	{id: "q-pdf-blank", raw: "pdf export produces blank pages", noise: 0.3,
		tags: []TagScore{{"export", 0.9}, {"pdf", 0.9}}},
	{id: "q-export-timeout", raw: "export times out on large projects", noise: 0.35,
		tags: []TagScore{{"export", 0.85}, {"performance", 0.6}}},
	{id: "q-pdf-font", raw: "exported pdf uses wrong font and clips titles", noise: 0.35,
		tags: []TagScore{{"pdf", 0.85}, {"ui", 0.4}}},
	{id: "q-csv-rows", raw: "csv export missing rows", noise: 0.35,
		tags: []TagScore{{"export", 0.8}}},
	{id: "q-stale-search", raw: "search shows stale results after renaming an issue", noise: 0.35,
		tags: []TagScore{{"search", 0.9}}},
	{id: "q-search-freeze", raw: "search box slow and freezes while typing", noise: 0.35,
		tags: []TagScore{{"search", 0.8}, {"performance", 0.7}}},
	{id: "q-fuzzy-rank", raw: "exact title match ranked below fuzzy matches", noise: 0.35,
		tags: []TagScore{{"search", 0.9}}},
	{id: "q-slow-dashboard", raw: "dashboard extremely slow for large organization", noise: 0.4,
		tags: []TagScore{{"performance", 0.85}, {"database", 0.5}}},
	{id: "q-nplus1", raw: "n+1 queries loading issue tags", noise: 0.35,
		tags: []TagScore{{"performance", 0.7}, {"database", 0.8}}},
	{id: "q-ios-crash", raw: "app crashes on launch on ios", noise: 0.3,
		tags: []TagScore{{"ios", 0.9}, {"mobile", 0.8}}},
	{id: "q-push-wrong", raw: "push notification opens the wrong issue", noise: 0.35,
		tags: []TagScore{{"notifications", 0.85}, {"ios", 0.6}}},
	{id: "q-keyboard-covers", raw: "keyboard hides the comment box on iphone", noise: 0.35,
		tags: []TagScore{{"ios", 0.8}, {"ui", 0.5}}},
	{id: "q-offline-lost", raw: "offline changes lost on mobile", noise: 0.4,
		tags: []TagScore{{"mobile", 0.85}}},
	{id: "q-dup-push", raw: "duplicate push notifications for one mention", noise: 0.35,
		tags: []TagScore{{"notifications", 0.9}}},
	{id: "q-outlook-dark", raw: "email broken in outlook dark mode", noise: 0.35,
		tags: []TagScore{{"email", 0.85}, {"ui", 0.4}}},
	{id: "q-invite-spam", raw: "invitation emails going to spam", noise: 0.35,
		tags: []TagScore{{"email", 0.8}}},
	{id: "q-deadlock", raw: "deadlock when updating issues in a transaction", noise: 0.35,
		tags: []TagScore{{"database", 0.85}, {"backend", 0.5}}},
	{id: "q-migration-fail", raw: "migration fails on postgres concurrent index", noise: 0.3,
		tags: []TagScore{{"migration", 0.9}, {"database", 0.7}}},
	{id: "q-replica-lag", raw: "stale counts from read replica lag", noise: 0.4,
		tags: []TagScore{{"database", 0.8}, {"performance", 0.5}}},
	{id: "q-tag-ui", raw: "ui", noise: 0.25,
		tags: []TagScore{{"ui", 0.7}}},
	{id: "q-tag-backend", raw: "backend", noise: 0.25,
		tags: []TagScore{{"backend", 0.8}}},
	{id: "q-badge-count", raw: "notification badge never clears", noise: 0.35,
		tags: []TagScore{{"notifications", 0.8}, {"ui", 0.5}}},
	{id: "q-export-email", raw: "scheduled export email has corrupted attachment", noise: 0.35,
		tags: []TagScore{{"export", 0.8}, {"email", 0.6}}},
}
