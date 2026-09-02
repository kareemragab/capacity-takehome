"""Put the seeded data into a known state for a demo, in one command.

    make up && make api      # in another terminal
    make demo

Afterwards, acting as "You":

    Pink   0 / 1     empty, and still unreachable
    Blue   3 / 3     Grace, Alan, Katherine
    Green  5 / 5     Barbara, Edsger, Radia, Margaret, Ada
    budget 8 / 8

    inbox: Ken wants to be a Pink

So the first click of the demo, accepting Ken, is refused by the shared budget
even though Pink is empty. That is rule 1 in one screen.
"""

import json
import sys
import urllib.error
import urllib.request

API = "http://localhost:8080/query"

BLUE = ["Grace", "Alan", "Katherine"]
GREEN = ["Barbara", "Edsger", "Radia", "Margaret", "Ada"]
ASKER = "Ken"
ME = "You"


def gql(query, variables=None, user=None):
    headers = {"Content-Type": "application/json"}
    if user:
        headers["X-User-Id"] = user
    req = urllib.request.Request(
        API, data=json.dumps({"query": query, "variables": variables or {}}).encode(), headers=headers
    )
    body = json.load(urllib.request.urlopen(req))
    if body.get("errors"):
        raise RuntimeError(body["errors"][0]["message"])
    return body["data"]


def clear(users):
    """Remove every contact and close every pending request, for everyone."""
    for uid in users.values():
        for c in gql("{ contacts { id } }", user=uid)["contacts"]:
            gql("mutation($id: ID!) { removeContact(contactId: $id) }", {"id": c["id"]}, uid)
        for r in gql("{ incomingRequests { id } }", user=uid)["incomingRequests"]:
            gql("mutation($id: ID!) { declineRequest(requestId: $id) { id } }", {"id": r["id"]}, uid)


def befriend(users, name, tier):
    """Have `name` ask to be my contact at `tier`, and accept it."""
    req = gql(
        "mutation($to: ID!, $tier: Tier!) { sendRequest(toUserId: $to, tier: $tier) { id } }",
        {"to": users[ME], "tier": tier},
        users[name],
    )["sendRequest"]
    gql("mutation($id: ID!) { acceptRequest(requestId: $id) { id } }", {"id": req["id"]}, users[ME])


def main():
    try:
        users = {u["name"]: u["id"] for u in gql("{ users { id name } }")["users"]}
    except urllib.error.URLError as e:
        sys.exit(f"No API on {API}. Run `make up` and `make api` first. ({e})")

    clear(users)
    for name in BLUE:
        befriend(users, name, "BLUE")
    for name in GREEN:
        befriend(users, name, "GREEN")
    gql(
        "mutation($to: ID!) { sendRequest(toUserId: $to, tier: PINK) { id } }",
        {"to": users[ME]},
        users[ASKER],
    )

    cap = gql("{ capacity { budgetUsed budgetCap tiers { tier used cap } } }", user=users[ME])["capacity"]
    print(f"acting as {ME}: budget {cap['budgetUsed']} / {cap['budgetCap']}")
    for t in cap["tiers"]:
        print(f"  {t['tier']:<6} {t['used']} / {t['cap']}")
    print(f"  inbox: {ASKER} is waiting, asking for Pink")


if __name__ == "__main__":
    main()
