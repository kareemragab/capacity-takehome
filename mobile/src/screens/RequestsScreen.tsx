import { useCallback, useMemo } from 'react';
import { RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useLoad, useRowActions } from '../hooks';
import { api } from '../queries';
import { TIER_LABEL, type Request, type Tier, type User } from '../types';
import { Button, Empty, Refusal, s, Section, TierPicker, TierTag } from '../ui';

/**
 * R6: the inbox. A failed accept says why, under the request it failed on.
 * R2: accept / decline. R1: send a request to a named tier.
 */
export function RequestsScreen({ me }: { me: string }) {
  const { data, error, loading, reload } = useLoad(() => api.requests(me), [me]);
  const { busy, refusals, run } = useRowActions(reload);

  const accept = useCallback((r: Request) => run(r.id, () => api.accept(me, r.id)), [me, run]);
  const decline = useCallback((r: Request) => run(r.id, () => api.decline(me, r.id)), [me, run]);
  const send = useCallback(
    (u: User, tier: Tier) => run(u.id, () => api.send(me, u.id, tier)),
    [me, run],
  );

  // People you can still ask: not you, not a contact, nothing pending either way.
  const askable = useMemo(() => {
    if (!data) return [];
    const taken = new Set<string>([me]);
    data.contacts.forEach((c) => taken.add(c.user.id));
    data.outgoingRequests.filter((r) => r.status === 'PENDING').forEach((r) => taken.add(r.to.id));
    data.incomingRequests.forEach((r) => taken.add(r.from.id));
    return data.users.filter((u) => !taken.has(u.id));
  }, [data, me]);

  return (
    <ScrollView
      contentContainerStyle={st.body}
      refreshControl={<RefreshControl refreshing={loading} onRefresh={reload} />}
    >
      {error && <Refusal text={error} />}

      {data && (
        <Section title="Inbox" right={<Text style={s.sub}>{data.incomingRequests.length} waiting</Text>}>
          {data.incomingRequests.length === 0 && <Empty text="Nothing waiting on you." />}
          {data.incomingRequests.map((r) => (
            <View key={r.id} style={s.row}>
              <View style={s.rowTop}>
                <Text style={s.name}>
                  {r.from.name} <Text style={s.sub}>wants to add you as</Text>
                </Text>
                <TierTag tier={r.tier} />
              </View>
              <View style={s.actions}>
                <Button label="Accept" tone="primary" disabled={busy === r.id} onPress={() => void accept(r)} />
                <Button label="Decline" disabled={busy === r.id} onPress={() => void decline(r)} />
              </View>
              <Refusal text={refusals[r.id]} />
            </View>
          ))}
        </Section>
      )}

      {data && (
        <Section title="Send a request">
          {askable.length === 0 && <Empty text="You've asked everyone there is." />}
          {askable.map((u) => (
            <View key={u.id} style={s.row}>
              <View style={s.rowTop}>
                <Text style={s.name}>{u.name}</Text>
                <Text style={s.sub}>pick a tier</Text>
              </View>
              <TierPicker disabled={busy === u.id} onPick={(tier) => void send(u, tier)} />
              <Refusal text={refusals[u.id]} />
            </View>
          ))}
        </Section>
      )}

      {data && (
        <Section title="Sent">
          {data.outgoingRequests.length === 0 && <Empty text="You haven't sent any yet." />}
          {data.outgoingRequests.map((r) => (
            <View key={r.id} style={[s.row, s.rowTop]}>
              <Text style={s.name}>
                {r.to.name} <Text style={s.sub}>as {TIER_LABEL[r.tier]}</Text>
              </Text>
              <Text style={[st.status, r.status === 'DECLINED' && st.declined, r.status === 'ACCEPTED' && st.accepted]}>
                {r.status.toLowerCase()}
              </Text>
            </View>
          ))}
        </Section>
      )}
    </ScrollView>
  );
}

const st = StyleSheet.create({
  body: { padding: 20, gap: 24, paddingBottom: 40 },
  status: { fontSize: 12, textTransform: 'uppercase', letterSpacing: 1, color: '#888' },
  declined: { color: '#9a3b2e' },
  accepted: { color: '#16605c' },
});
