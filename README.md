# Valorant-League-Of-Legends-Scouting-Report-Analyzer

> Comprehensive REST API for esports team analysis and scouting powered by Grid.gg API

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)

## Quick Start

```bash
# 1. Clone repository
git clone <your-repo-url>
cd esports-scouting-backend

# 2. Set environment variables
export GRID_API_KEY="your_hackathon_api_key"
export REDIS_URL="redis://localhost:6379"
export DATABASE_URL="postgresql://user:pass@localhost:5432/esports"
export PORT="8080"

# 3. Run
go run cmd/api/main.go
```

Server starts at `http://localhost:8080`

---

## 📚 API Endpoints

### Core Endpoints

#### 1. Health Check
```http
GET /health
```

**Response:**
```json
{
  "status": "ok",
  "postgres": true,
  "redis": true,
  "grid_api": true,
  "timestamp": "2026-01-23T10:30:00Z"
}
```

---

#### 2. Compare Teams
```http
GET /api/v1/compare?team1={name}&team2={name}&title={title}&timeWindow={window}
```

**Parameters:**
- `team1` (required): First team name
- `team2` (required): Second team name
- `title` (required): `valorant` or `lol`
- `timeWindow` (optional): `LAST_WEEK` | `LAST_MONTH` | `LAST_3_MONTHS` (default) | `LAST_6_MONTHS` | `LAST_YEAR`
- `tournamentIds` (optional): Comma-separated IDs (auto-selected if omitted)

**Example:**
```bash
curl "http://localhost:8080/api/v1/compare?team1=T1&team2=Gen.G%20Esports&title=lol"
```

**Response:**
```json
{
  "team1": {
    "name": "T1",
    "stats": {
      "winRate": 0.65,
      "kdRatio": 1.15,
      "kills": { "avg": 18.5, "total": 370 },
      "deaths": { "avg": 16.0, "total": 320 },
      "currentStreak": { "type": "win", "count": 3 },
      "confidence": {
        "level": "HIGH",
        "sampleSize": 20,
        "reasoning": "Sample size is adequate (20 matches)",
        "reliabilityScore": 85
      }
    }
  },
  "team2": { "..." },
  "advantages": {
    "team1": ["Higher win rate (+10%)", "Better K/D (+0.2)"],
    "team2": []
  },
  "dataQuality": {
    "team1Matches": 20,
    "team2Matches": 18,
    "timeRange": "LAST_3_MONTHS"
  },
  "warnings": [],
  "recentTrends": {
    "team1HasAlerts": true,
    "team1Alerts": [{
      "type": "POSITIVE_SHIFT",
      "severity": "HIGH",
      "message": "Win rate increased by 29%",
      "context": "Team is performing significantly better recently"
    }]
  }
}
```

---

#### 3. Team Trends (Recency Bias Detection)
```http
GET /api/v1/team/trends?name={team}&title={title}
```

**Parameters:**
- `name` (required): Team name (case-insensitive)
- `title` (required): `valorant` or `lol`
- `tournamentIds` (optional): Filter by tournaments

**Example:**
```bash
curl "http://localhost:8080/api/v1/team/trends?name=Sentinels&title=valorant"
```

**Response:**
```json
{
  "team": "Sentinels",
  "title": "valorant",
  "overall": {
    "timeWindow": "LAST_3_MONTHS",
    "winRate": 0.58,
    "kdRatio": 1.15,
    "matches": 24
  },
  "recent": {
    "timeWindow": "LAST_MONTH",
    "winRate": 0.75,
    "kdRatio": 1.35,
    "matches": 4
  },
  "alerts": [
    {
      "type": "POSITIVE_SHIFT",
      "severity": "HIGH",
      "message": "Win rate increased by 29% in recent matches",
      "context": "Team is performing significantly better recently"
    }
  ],
  "confidence": {
    "level": "MEDIUM",
    "sampleSize": 4,
    "reasoning": "Recent sample expanded to LAST_MONTH (4 matches) for accuracy",
    "reliabilityScore": 65
  }
}
```

---

#### 4. Comprehensive Scouting Report
```http
GET /api/v1/scouting-report?opponent={name}&myTeam={name}&title={title}
```

**Parameters:**
- `opponent` (required): Opponent team name
- `myTeam` (required): Your team name
- `title` (required): `valorant` or `lol`
- `timeWindow` (optional): Default `LAST_3_MONTHS`
- `tournamentIds` (optional): Auto-selected if omitted

**Example:**
```bash
curl "http://localhost:8080/api/v1/scouting-report?opponent=G2%20Esports&myTeam=Cloud9&title=valorant"
```

**Response:**
```json
{
  "reportId": "550e8400-e29b-41d4-a716-446655440000",
  "generatedAt": "2026-01-23T10:30:00Z",
  "matchup": {
    "opponent": "G2 Esports",
    "yourTeam": "Cloud9",
    "title": "valorant"
  },
  "comparison": {
    "// Full comparison data from /compare endpoint"
  },
  "trends": {
    "opponent": {
      "// Trend analysis for opponent"
    },
    "yourTeam": {
      "// Trend analysis for your team"
    }
  },
  "metaContext": {
    "opponentVsMeta": [
      "G2 Esports plays standard compositions",
      "Meta analysis requires additional API access"
    ],
    "yourTeamVsMeta": ["..."],
    "recommendations": [
      "Focus on individual team performance metrics"
    ]
  },
  "keyInsights": [
    {
      "priority": "HIGH",
      "icon": "🔴",
      "message": "Opponent win rate up 29% recently - prepare for strong performance"
    },
    {
      "priority": "MEDIUM",
      "icon": "🟢",
      "message": "You have better K/D ratio (+0.25) - maintain aggressive plays"
    }
  ],
  "confidence": {
    "level": "HIGH",
    "sampleSize": 21,
    "reasoning": "Based on 42 matches analyzed across both teams",
    "reliabilityScore": 82
  },
  "cacheStatus": {
    "fromCache": false,
    "age": "1.234s"
  }
}
```

**Key Features:**
- Combines comparison, trends, and meta analysis
- Prioritized actionable insights (HIGH/MEDIUM/LOW)
- Parallel data fetching (<5s response time with cache)
- 1-hour cache for optimal performance
- Graceful degradation if any data source fails

---

#### 5. Team Search (Autocomplete)
```http
GET /api/v1/teams/search?q={query}&title={title}
```

**Parameters:**
- `q` (required): Search query (minimum 1 character)
- `title` (required): `valorant` or `lol`

**Example:**
```bash
curl "http://localhost:8080/api/v1/teams/search?q=cloud&title=valorant"
```

**Response:**
```json
{
  "query": "cloud",
  "results": [
    {
      "name": "Cloud9",
      "displayName": "Cloud9",
      "title": "valorant",
      "relevance": 100
    }
  ],
  "count": 1
}
```

**Relevance Scoring:**
- `100`: Exact prefix match (starts with query)
- `75`: Starts with same letter
- `50`: Contains query anywhere

---

#### 6. : Meta Analysis
```http
GET /api/v1/meta?title={title}&tournamentId={id}
```

**Parameters:**
- `title` (required): `valorant` or `lol`
- `tournamentId` (optional): Specific tournament

**Note:** Currently returns placeholder data as Grid.gg hackathon tier doesn't include pick/ban data API.

**Response:**
```json
{
  "error": "Meta analysis requires Grid.gg pick/ban data API (not available in current tier)",
  "message": "Use team statistics endpoints for performance analysis"
}
```

---

### Discovery Endpoints

#### Get Available Titles
```http
GET /api/v1/titles
```

**Response:**
```json
{
  "titles": [
    {
      "id": "6",
      "name": "Valorant",
      "slug": "valorant",
      "description": "Tactical FPS by Riot Games"
    },
    {
      "id": "3",
      "name": "League of Legends",
      "slug": "lol",
      "aliases": ["lol", "leagueoflegends"],
      "description": "MOBA by Riot Games"
    }
  ],
  "count": 2
}
```

---

#### Get Available Tournaments
```http
GET /api/v1/tournaments?title={title}
```

**Example:**
```bash
curl "http://localhost:8080/api/v1/tournaments?title=valorant"
```

**Response:**
```json
{
  "title": "valorant",
  "tournaments": [
    {"id": "757371", "name": "VCT Americas - Kickoff 2024"},
    {"id": "757481", "name": "VCT Americas - Stage 1 2024"},
    {"id": "774782", "name": "VCT Americas - Stage 2 2024"},
    {"id": "775516", "name": "VCT Americas - Kickoff 2025"},
    {"id": "800675", "name": "VCT Americas - Stage 1 2025"},
    {"id": "826660", "name": "VCT Americas - Stage 2 2025"}
  ],
  "count": 6
}
```

---

#### Get Available Teams
```http
GET /api/v1/teams?title={title}&validateData={true|false}
```

**Parameters:**
- `title` (required): `valorant` or `lol`
- `validateData` (optional):
    - Default/true: Only shows teams with accessible Series State data ✅ **RECOMMENDED**
    - false: Shows all teams (some may fail when queried)

**Example:**
```bash
# Default: Only teams with data (recommended)
curl "http://localhost:8080/api/v1/teams?title=lol"

# Legacy: All teams (including those without data)
curl "http://localhost:8080/api/v1/teams?title=lol&validateData=false"
```

**Response:**
```json
{
  "title": "lol",
  "teams": [
    "T1", "Gen.G Esports", "Fnatic", "G2 Esports",
    "Cloud9 Kia", "Team Vitality"
  ],
  "count": 6,
  "note": "Only teams with accessible Series State data. Use these names in other endpoints."
}
```

**⚠️ Important:** By default, this endpoint validates data access by sampling one series per team. This takes ~30 seconds but ensures all returned teams are queryable.

---

## 🎯 Key Features

### 1. Graduated Fallback System 
Automatically expands time windows when insufficient data:
```
LAST_WEEK (0 matches) → LAST_MONTH (4 matches) ✓
```
No more misleading identical comparisons!

### 2. Data Access Validation 
`/api/v1/teams` endpoint validates Series State data access:
- Only returns teams you can actually query
- Prevents "500 Internal Server Error" from inaccessible teams
- Proper `404 Not Found` errors with helpful messages

### 3. Proper Error Codes 
- `404`: Team not found or insufficient data (with clear reason)
- `400`: Missing/invalid parameters
- `500`: Only for unexpected server errors
- `504`: API timeout

**Example Error Response:**
```json
{
  "error": "insufficient data for team 'Sentinels': no recent matches found (last match: 2025-08-30)",
  "team": "Sentinels",
  "reason": "no recent matches found",
  "title": "valorant",
  "message": "Team has insufficient data available. Try a team with recent matches."
}
```

### 4. Confidence Scoring
Every stat includes reliability metrics:
- **HIGH**: ≥15 matches, 85+ reliability score
- **MEDIUM**: 7-14 matches, 60-84 score
- **LOW**: <7 matches, <60 score

### 5. Smart Caching
- Comparison: 1 hour TTL
- Trends: 3 hours TTL
- Teams list (validated): 6 hours TTL
- Scouting reports: 1 hour TTL

**Performance:**
- Cache hit: ~100-300ms
- Cache miss: 5-10 seconds (Grid.gg API latency)
- Team validation: ~30 seconds (one-time per 6 hours)

### 6. Parallel Data Fetching
Scouting report fetches all data simultaneously:
- Comparison data
- Trends for both teams
- Meta context
- Total time: ~5-8 seconds (uncached)

### 7. Title Validation 
All endpoints validate `title` parameter:
- Must be `valorant` or `lol`
- Returns clear error if teams from different games are compared
- Prevents confusing "team not found" errors

---

## 🛠️ Environment Variables

### Required
```bash
PORT=Your desired port
REDIS_URL=Your Redis Url
GRID_API_KEY=Your Grid Api Key
DATABASE_URL=Your Neon Database Url
NEON_API_KEY=Your Neon APi Key
TRUSTED_PROXIES=Your desired proxy
datasource.url=Your Neon datasource url
datasource.username=Your neon db username
datasource.password=Your neon db password                   
```

---

## 📦 Dependencies

```
github.com/gin-gonic/gin              # Web framework
github.com/machinebox/graphql         # GraphQL client
github.com/go-redis/redis/v8          # Redis client
github.com/lib/pq                     # PostgreSQL driver
github.com/google/uuid                # UUID generation
```

Install all dependencies:
```bash
go mod download
```

## ⚡ Performance Optimization

### Response Time Targets
- ✅ Health check: <100ms
- ✅ Cached requests: <300ms
- ✅ Scouting report (cached): <50ms
- ✅ Scouting report (uncached): <10s

### Optimization Strategies
1. **Redis caching** with appropriate TTLs
2. **Parallel fetching** for scouting reports
3. **Client-side filtering** to minimize API calls
4. **Graceful degradation** for non-critical data

### Rate Limiting
Grid.gg API has rate limits:
- Be respectful of API quotas
- Cache aggressively
- Use Redis for request deduplication

---

## 🐛 Common Issues

### Issue: "Team not found" or Wrong Game Title
**Cause:** Comparing teams from different games or typo in title parameter  
**Solution:**
```bash
# ✅ CORRECT: Both teams + correct title
curl "http://localhost:8080/api/v1/compare?team1=Cloud9&team2=Sentinels&title=valorant"

# ❌ WRONG: Mixing Valorant team (NRG) with LoL title
curl "http://localhost:8080/api/v1/compare?team1=NRG&team2=Gen.G&title=lol"

# ✅ FIX: Use correct title for both teams
curl "http://localhost:8080/api/v1/compare?team1=FURIA&team2=NRG&title=valorant"
```

**Always verify:**
1. Get available teams for your title first:
   ```bash
   curl "http://localhost:8080/api/v1/teams?title=valorant"
   ```
2. Use exact team names from that list
3. Ensure `title` parameter matches the game you want

### Issue: Teams showing in `/teams` but failing in `/compare` or `/trends`
**Cause:** Team validation now filters by data access, but some edge cases remain  
**Solution:**
- The `/api/v1/teams` endpoint now validates data access by default
- Only teams with Series State data are shown
- If a team still fails, it means recent data is unavailable (no matches in last 6 months)

**Example Response for Teams with No Recent Data:**
```json
{
  "error": "insufficient data for team 'Sentinels': no recent matches found (last match: 2025-08-30)",
  "team": "Sentinels",
  "reason": "no recent matches found",
  "message": "Team has insufficient data available. Try a team with recent matches."
}
```
**Status Code:** `404 Not Found` (not 500)

---

## 📊 Data Constraints

### Grid.gg Hackathon Tier
- **Data Range:** Last 2 years only (Jan 2024 - Jan 2026)
- **Tournaments:** Pre-selected VCT Americas & LCK/LCS/LEC/LPL
- **Teams:** ~12-15 Valorant, ~20-30 LoL teams
- **API Endpoints:** Central Data + Series State only

### Missing Features
- ❌ Pick/ban data (meta analysis placeholder only)
- ❌ Player-level detailed stats (available in API but not exposed)
- ❌ Head-to-head matchup history (requires cross-referencing)

---

## 👥 Contributing

1. Fork the repository
2. Create feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open Pull Request

---

## 🙏 Acknowledgments
-**ALL GLORY TO GOD**
- **Grid.gg** for hackathon API access
- **Cloud9 & JetBrains** for organizing the hackathon
- **Render** for easy deployment platform

---
