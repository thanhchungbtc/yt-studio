package entity

// Pool is a global concurrency pool: video A's chapter 3 competes with video
// B's chapter 40 for the same slots.
type Pool string

// The complete set of pools. A task acquires exactly one slot in exactly one
// pool and holds it for the duration of the provider call.
const (
	// PoolLLM covers blueprint, script, slide-prompt priming and metadata.
	PoolLLM Pool = "llm"
	// PoolTTS covers narration synthesis.
	PoolTTS Pool = "tts"
	// PoolImage covers slide generation — the longest pole in the pipeline.
	PoolImage Pool = "image"
	// PoolCompose covers per-chapter clips and the final concat.
	PoolCompose Pool = "compose"
	// PoolCache covers the per-chapter slide-prompt reads against the coalesced
	// batch, which must not occupy a real LLM slot.
	PoolCache Pool = "cache"
	// PoolUpload covers the YouTube upload.
	PoolUpload Pool = "upload"
)

// AllPools lists every pool in index order. The scheduler indexes fixed-size
// arrays by Pool.Index(), so this order is load-bearing: append only.
var AllPools = [...]Pool{PoolLLM, PoolTTS, PoolImage, PoolCompose, PoolCache, PoolUpload}

// NumPools is the size of the fixed arrays the scheduler allocates once.
const NumPools = len(AllPools)

// Index returns the dense array index of the pool, or -1 if unknown.
// Allocation-free, so it is safe on the dispatch hot path.
func (p Pool) Index() int {
	switch p {
	case PoolLLM:
		return 0
	case PoolTTS:
		return 1
	case PoolImage:
		return 2
	case PoolCompose:
		return 3
	case PoolCache:
		return 4
	case PoolUpload:
		return 5
	default:
		return -1
	}
}

// Valid reports whether the pool is one of the known constants.
func (p Pool) Valid() bool { return p.Index() >= 0 }

// String returns the underlying text of the pool name.
func (p Pool) String() string { return string(p) }

// DefaultPoolLimit is the seeded limit for each pool; each is a settings row
// changeable without a restart.
func DefaultPoolLimit(p Pool) int {
	switch p {
	case PoolLLM, PoolTTS, PoolImage, PoolCompose:
		return 2
	case PoolCache:
		// Cache reads are cheap and must never be the bottleneck.
		return 32
	case PoolUpload:
		return 1
	default:
		return 1
	}
}

// MaxPoolLimit is the capacity a pool's semaphore is created with; a limit
// change moves ballast inside this ceiling rather than rebuilding it.
const MaxPoolLimit = 256
