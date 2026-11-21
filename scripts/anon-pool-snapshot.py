#!/usr/bin/env python3
"""Anonymise an `agx pool pick --json` envelope into a pool test fixture.

Usage (cache only, so nothing is probed and no quota is spent):

    agx pool pick --id auto --cached --json | python3 -I scripts/anon-pool-snapshot.py > pool/testdata/pick-YYYY-MM-DD-anon.json

Every member name is replaced by a made-up one in report order (alice, bob,
carol, ...), every account by NAME@example.com, and every other occurrence of
a real name inside a reason or note is replaced too. Settings, warnings,
sources and cache ages are dropped: the fixture keeps only what the rules
read (group, status, usage, resets, caps and per-model windows) and what the
pick printed (order, ranks, reason), so pool/snapshot_pick_test.go can
replay it with pool.Select. Read the output before committing it: it must
name no real person, account, machine or organisation.
"""

import json
import re
import sys

# The simulator's made-up names (internal/poolsim/scenario.go), so a
# fixture can also become a scenario file.
FAKE = [
    "alice", "bob", "carol", "derek", "erin", "frank", "grace", "heidi", "ivan", "judy", "ken", "liam", "mallory",
    "niaj", "olivia", "peggy", "quinn", "rupert", "sybil", "trent", "uma", "victor", "walter", "xena", "yara", "zoe",
]

KEEP = [
    "group", "state", "rank", "eligible", "enabled", "status", "short",
    "week", "weekly_resets_at", "hours_to_reset", "room_percent", "caps",
    "week_cap_configured", "urgency_percent_per_hour", "scoped", "reasons",
    "notes", "sessions",
]


def main() -> None:
    env = json.load(sys.stdin)
    data = env["data"]
    names = {}
    for i, c in enumerate(data["candidates"]):
        names[c["name"]] = FAKE[i]
    # Longest first, so a name inside another is not half replaced.
    pattern = re.compile(r"\b(" + "|".join(sorted(map(re.escape, names), key=len, reverse=True)) + r")\b")

    def clean(v):
        if isinstance(v, str):
            return pattern.sub(lambda m: names[m.group(1)], v)
        if isinstance(v, list):
            return [clean(x) for x in v]
        if isinstance(v, dict):
            return {k: clean(x) for k, x in v.items()}
        return v

    cands = []
    for c in data["candidates"]:
        out = {"name": names[c["name"]], "account": names[c["name"]] + "@example.com"}
        for k in KEEP:
            if k in c:
                out[k] = clean(c[k])
        cands.append(out)
    sel = data.get("selected")
    if sel:
        sel = {"name": names.get(sel["name"], sel["name"]), "reason": clean(sel["reason"]), "sessions": sel.get("sessions", 0)}
    doc = {
        "about": "agx pool pick --cached --json, anonymised by scripts/anon-pool-snapshot.py",
        "now": data["now"],
        "order": data["order"],
        "shared_borrow_window_hours": data.get("shared_borrow_window_hours"),
        "selected": sel,
        "candidates": cands,
    }
    json.dump(doc, sys.stdout, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
