"""Walks every mutation against a running API (make up, make api) and prints
each refusal sentence. Acts as the seeded users via X-User-Id. Safe to rerun:
it starts by clearing "You"'s contacts. python3 scripts/smoke.py
"""
import json, urllib.request
API="http://localhost:8080/query"
def gql(q, v=None, user=None):
    body=json.dumps({"query":q,"variables":v or {}}).encode()
    req=urllib.request.Request(API, data=body, headers={"Content-Type":"application/json", **({"X-User-Id":user} if user else {})})
    return json.load(urllib.request.urlopen(req))
users={u["name"]:u["id"] for u in gql("{ users { id name } }")["data"]["users"]}
me=users["You"]
names=["Ada","Grace","Alan","Katherine","Barbara","Edsger","Radia","Ken","Margaret"]
def show(label,res):
    if res.get("errors"): print(f"{label}: REFUSED -> {res['errors'][0]['message']}  {res['errors'][0].get('extensions')}")
    else: print(f"{label}: ok -> {json.dumps(res['data'])[:160]}")
# clean slate: remove all my contacts
for c in gql("{ contacts { id } }", user=me)["data"]["contacts"]:
    gql("mutation($id:ID!){ removeContact(contactId:$id) }", {"id":c["id"]}, me)
show("no header", gql("{ capacity { budgetUsed } }"))
show("self", gql("mutation($to:ID!){ sendRequest(toUserId:$to, tier:PINK){ id } }", {"to":me}, me))
# fill 1 pink, 3 blue, 3 green by asking others to send me requests and accepting
plan=[("Ada","PINK"),("Grace","BLUE"),("Alan","BLUE"),("Katherine","BLUE"),("Barbara","GREEN"),("Edsger","GREEN"),("Radia","GREEN")]
for n,t in plan:
    r=gql("mutation($to:ID!,$t:Tier!){ sendRequest(toUserId:$to, tier:$t){ id } }", {"to":me,"t":t}, users[n])
    rid=r["data"]["sendRequest"]["id"]
    show(f"accept {n} into {t}", gql("mutation($id:ID!){ acceptRequest(requestId:$id){ id tier user{name} } }", {"id":rid}, me))
show("capacity 7/8", gql("{ capacity { budgetUsed budgetCap tiers { tier used cap } } }", user=me))
# Rule 1b: Ken -> me PINK (pink full, budget has 1) -> tier refusal
r=gql("mutation($to:ID!){ sendRequest(toUserId:$to, tier:PINK){ id } }", {"to":me}, users["Ken"]); ken=r["data"]["sendRequest"]["id"]
show("R1b accept Ken PINK (pink full, budget room)", gql("mutation($id:ID!){ acceptRequest(requestId:$id){ id } }", {"id":ken}, me))
# Rule 2: pending holds no seat -> I can still send several with one seat
for n in ["Margaret"]:
    show(f"send to {n} GREEN with 1 seat free", gql("mutation($to:ID!){ sendRequest(toUserId:$to, tier:GREEN){ id status } }", {"to":users[n]}, me))
show("reverse duplicate (Ken sends again)", gql("mutation($to:ID!){ sendRequest(toUserId:$to, tier:BLUE){ id } }", {"to":me}, users["Ken"]))
show("me -> Ken while Ken's request is pending", gql("mutation($to:ID!){ sendRequest(toUserId:$to, tier:BLUE){ id } }", {"to":users["Ken"]}, me))
# take the 8th seat: Margaret accepts my request
inbox=gql("{ incomingRequests { id from{name} tier } }", user=users["Margaret"])["data"]["incomingRequests"]
show("Margaret accepts me", gql("mutation($id:ID!){ acceptRequest(requestId:$id){ id tier user{name} } }", {"id":inbox[0]["id"]}, users["Margaret"]))
show("capacity 8/8", gql("{ capacity { budgetUsed budgetCap tiers { tier used cap } } }", user=me))
# Rule 1: budget before sub-cap: Ken's PINK request now fails on the budget
show("R1 accept Ken PINK at 8/8", gql("mutation($id:ID!){ acceptRequest(requestId:$id){ id } }", {"id":ken}, me))
# Rule 2 at 8/8: sending refused
show("send at 8/8", gql("mutation($to:ID!){ sendRequest(toUserId:$to, tier:GREEN){ id } }", {"to":users["Ken"]}, me))
# Rule 3: move at 8/8 Green->Blue (blue full) refused on tier; Blue->Pink refused; Green -> ... 
contacts=gql("{ contacts { id tier user{name} } }", user=me)["data"]["contacts"]
byname={c["user"]["name"]:c for c in contacts}
show("R3 move Barbara GREEN->BLUE (blue 3/3)", gql("mutation($id:ID!,$t:Tier!){ moveContact(contactId:$id, tier:$t){ id tier } }", {"id":byname["Barbara"]["id"],"t":"BLUE"}, me))
show("R3 move Grace BLUE->GREEN (green 4/5) at 8/8 budget", gql("mutation($id:ID!,$t:Tier!){ moveContact(contactId:$id, tier:$t){ id tier user{name} } }", {"id":byname["Grace"]["id"],"t":"GREEN"}, me))
show("R3 move Grace GREEN->GREEN", gql("mutation($id:ID!,$t:Tier!){ moveContact(contactId:$id, tier:$t){ id tier } }", {"id":byname["Grace"]["id"],"t":"GREEN"}, me))
# R4 remove frees both sides
show("R4 remove Ada", gql("mutation($id:ID!){ removeContact(contactId:$id) }", {"id":byname["Ada"]["id"]}, me))
show("Ada's contacts after", gql("{ contacts { user{name} tier } capacity { budgetUsed } }", user=users["Ada"]))
show("R1 retry Ken PINK after freeing (pink now 0/1)", gql("mutation($id:ID!){ acceptRequest(requestId:$id){ id tier user{name} } }", {"id":ken}, me))
show("decline nonexistent", gql("mutation($id:ID!){ declineRequest(requestId:$id){ id } }", {"id":"6a986b9996dcd38e6a9c2400"}, me))
show("bad id", gql("mutation($id:ID!){ declineRequest(requestId:$id){ id } }", {"id":"nope"}, me))
show("outgoing", gql("{ outgoingRequests { to{name} tier status } }", user=me))
show("final capacity", gql("{ capacity { budgetUsed budgetCap tiers { tier used cap } } }", user=me))
