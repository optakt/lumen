#!/usr/bin/env bash
# Lumen Pi session demo
# Exercises the four agent tools (lumen_assert, lumen_correct, lumen_query, lumen_bio)
# against a running lumen-server to simulate what a Pi session looks like.
#
# Prerequisites:
#   go install github.com/optakt/lumen/cmd/lumen-server@latest
#   lumen-server -addr :3737 -db /tmp/demo.db

set -euo pipefail

BASE="${LUMEN_URL:-http://localhost:3737}/v1"

echo "=== Lumen Pi Session Demo ==="
echo "Server: $BASE"
echo

curl -sf "$BASE/health" > /dev/null || { echo "error: lumen-server not reachable at $BASE"; exit 1; }
echo "Server healthy."
echo

# ─── Turn 1: Initial claim ────────────────────────────────────────────────
echo "--- Turn 1: Agent calls lumen_assert ---"
echo "  Claim: 'GWT has the strongest empirical support'"
echo
R1=$(curl -sf -XPOST "$BASE/self/claim" -H 'Content-Type: application/json' \
  -d '{"kind":"derived","content":"Global Workspace Theory has the strongest current empirical support among consciousness theories","confidence":0.72}')
echo "$R1" | python3 -m json.tool
ID1=$(echo "$R1" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

echo
echo "  Claim: 'IIT remains viable'"
R2=$(curl -sf -XPOST "$BASE/self/claim" -H 'Content-Type: application/json' \
  -d '{"kind":"asserted","content":"Integrated Information Theory remains a viable framework despite criticism","confidence":0.55}')
echo "$R2" | python3 -m json.tool
ID2=$(echo "$R2" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

# ─── Turn 2: Correction ───────────────────────────────────────────────────
echo
echo "--- Turn 2: Principal pushes back on IIT. Agent calls lumen_correct ---"
echo "  Retracts: $ID2"
echo "  New claim: 'IIT faces significant challenges from Cogitate results'"
echo
R3=$(curl -sf -XPOST "$BASE/self/correct" -H 'Content-Type: application/json' \
  -d "{\"replaces_id\":\"$ID2\",\"content\":\"IIT faces significant challenges from the Cogitate results; its viability is now in doubt\",\"confidence\":0.35,\"reason\":\"Cogitate 2023 showed prefrontal cortex not necessary for consciousness, contrary to IIT prediction\"}")
echo "$R3" | python3 -m json.tool
ID3=$(echo "$R3" | python3 -c "import sys,json; print(json.load(sys.stdin)['new_id'])")

# ─── Turn 3: lumen_query ─────────────────────────────────────────────────
echo
echo "--- Turn 3: Agent calls lumen_query (min_confidence=0.3) ---"
echo
curl -sf "$BASE/context?min_confidence=0.3&max=10"

# ─── Turn 4: lumen_bio ───────────────────────────────────────────────────
echo
echo "--- Turn 4: Agent calls lumen_bio on GWT claim ---"
echo "  Belief: $ID1"
echo
curl -sf "$BASE/explain/$ID1" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('explanation', d))
"

echo
echo "=== Demo complete ==="
echo
echo "The corrected IIT belief ($ID2) is now suspect (marked ⚠)."
echo "GWT ($ID1) and the corrected IIT claim ($ID3) remain active."
