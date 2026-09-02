"""Slice the Claude Code session file from the first assessment message to the
end, write the raw jsonl and a readable markdown rendering. Nothing is
summarised or reordered; only the assistant's private memory notes that the
harness injected into the first message are redacted (personal data, unrelated
to the exercise), and screenshots are noted rather than embedded.
"""
import json, re, sys

SRC = '/Users/kareemhassan/.claude/projects/-Users-kareemhassan-Desktop-upWork/17790462-78b6-4ad0-b724-b24801982e0f.jsonl'
OUT_DIR = sys.argv[1]
START = int(sys.argv[2])
DRY = len(sys.argv) > 3 and sys.argv[3] == '--dry'
# (first, last) line ranges left out: messages about interview logistics, not the exercise.
SKIP = [(5621, 5673)]

MEM_RE = re.compile(r"Contents of /Users/kareemhassan/\.claude/projects/[^\n]*memory/MEMORY\.md[^\n]*\n(?:.*\n)*?(?=# userEmail)", re.M)
EMAIL_RE = re.compile(r"The user's email address is [^\n]*\n")

def redact(text: str) -> str:
    text = MEM_RE.sub("[redacted: the assistant's personal memory notes about me, injected by the tool, unrelated to this exercise]\n", text)
    text = EMAIL_RE.sub("[redacted: email]\n", text)
    for frag in ("kareemragab334", "krhassan334", "kareemragab33@"):
        text = text.replace(frag, "[redacted]")
    return text

def deep_redact(x):
    if isinstance(x, str):
        return redact(x)
    if isinstance(x, list):
        return [deep_redact(v) for v in x]
    if isinstance(x, dict):
        return {k: deep_redact(v) for k, v in x.items()}
    return x

def block_text(c):
    if isinstance(c, str):
        return c
    out = []
    for part in c:
        if not isinstance(part, dict):
            continue
        t = part.get('type')
        if t == 'text':
            out.append(part.get('text', ''))
        elif t == 'image':
            out.append('[image]')
    return '\n'.join(out)

entries = []
with open(SRC) as f:
    for i, line in enumerate(f):
        if i < START:
            continue
        if any(a <= i <= b for a, b in SKIP):
            continue
        o = json.loads(line)
        if o.get('type') not in ('user', 'assistant'):
            continue
        entries.append((i, line, o))

humans = []
md = ["# Agent session transcript\n",
      "Claude Code session, rendered from the raw `session.jsonl` next to this file. ",
      "Human turns are marked **Human**; the agent's replies **Assistant**; every tool the agent ran is shown with its input and the output it got back. Order is the original order. Nothing is summarised.\n"]
raw_lines = []
marked = set()
for i, line, o in entries:
    for a, b in SKIP:
        if i > b and (a, b) not in marked:
            marked.add((a, b))
            md.append(f"\n> **[cut]** Session lines {a}-{b} are left out: a few messages between me and the agent about the interview format, not about the exercise. See `transcripts/README.md`.\n")
    msg = o.get('message', {})
    role = msg.get('role')
    content = msg.get('content')
    ts = o.get('timestamp', '')
    if role == 'user':
        if isinstance(content, list) and any(isinstance(p, dict) and p.get('type') == 'tool_result' for p in content):
            for p in content:
                if not isinstance(p, dict):
                    continue
                if p.get('type') == 'tool_result':
                    body = block_text(p.get('content', ''))
                    md.append(f"\n<details><summary><b>Tool result</b> <code>{ts}</code></summary>\n\n```text\n{redact(body)}\n```\n\n</details>\n")
                elif p.get('type') == 'text':
                    md.append(f"\n**Human** `{ts}`\n\n{redact(p.get('text',''))}\n")
                    humans.append((i, ts, p.get('text','')))
        else:
            text = redact(block_text(content))
            md.append(f"\n---\n\n**Human** `{ts}`\n\n{text}\n")
            humans.append((i, ts, block_text(content)))
        raw_lines.append(json.dumps(deep_redact(o), ensure_ascii=False) + '\n')
    elif role == 'assistant':
        for p in content if isinstance(content, list) else [{'type': 'text', 'text': str(content)}]:
            if not isinstance(p, dict):
                continue
            t = p.get('type')
            if t == 'text' and p.get('text', '').strip():
                md.append(f"\n**Assistant** `{ts}`\n\n{p['text']}\n")
            elif t == 'thinking' and p.get('thinking', '').strip():
                md.append(f"\n<details><summary><i>Assistant thinking</i> <code>{ts}</code></summary>\n\n{p['thinking']}\n\n</details>\n")
            elif t == 'tool_use':
                inp = redact(json.dumps(p.get('input', {}), ensure_ascii=False, indent=2))
                md.append(f"\n**Tool call** `{p.get('name')}` `{ts}`\n\n```json\n{inp}\n```\n")
        raw_lines.append(json.dumps(deep_redact(o), ensure_ascii=False) + '\n')

if DRY:
    print(f"{len(entries)} entries, {len(humans)} human turns")
    for i, ts, text in humans:
        one = ' '.join(text.split())
        print(f"- L{i} {ts[11:19]}: {one[:160]}")
else:
    with open(f"{OUT_DIR}/session.md", 'w') as f:
        f.write('\n'.join(md))
    with open(f"{OUT_DIR}/session.jsonl", 'w') as f:
        f.writelines(raw_lines)
    print(f"wrote {len(entries)} entries")
