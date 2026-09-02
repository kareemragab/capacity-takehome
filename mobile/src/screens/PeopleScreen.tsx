import { useCallback, useState } from 'react';
import { RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useLoad, useRowActions } from '../hooks';
import { api } from '../queries';
import { TIER_COLOR, TIER_LABEL, TIERS, type Contact, type Tier } from '../types';
import { Button, Counter, Empty, Refusal, s, Section, TierPicker } from '../ui';

/**
 * R5: contacts by tier with live used / cap and the shared budget.
 * R3: re-file by tapping another tier. R4: remove, with a second tap to confirm.
 */
export function PeopleScreen({ me }: { me: string }) {
  const { data, error, loading, reload } = useLoad(() => api.people(me), [me]);
  const { busy, refusals, run } = useRowActions(reload);
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null);

  const move = useCallback(
    (c: Contact, tier: Tier) => run(c.id, () => api.move(me, c.id, tier)),
    [me, run],
  );
  const remove = useCallback(
    (c: Contact) => {
      if (confirmRemove !== c.id) {
        setConfirmRemove(c.id);
        return;
      }
      setConfirmRemove(null);
      void run(c.id, () => api.remove(me, c.id));
    },
    [confirmRemove, me, run],
  );

  const byTier = (t: Tier) => (data?.contacts ?? []).filter((c) => c.tier === t);
  const capOf = (t: Tier) => data?.capacity.tiers.find((x) => x.tier === t);

  return (
    <ScrollView
      contentContainerStyle={st.body}
      refreshControl={<RefreshControl refreshing={loading} onRefresh={reload} />}
    >
      {error && <Refusal text={error} />}

      {data && (
        <View style={st.budget}>
          <View style={s.sectionHead}>
            <Text style={st.budgetTitle}>Shared budget</Text>
            <Counter used={data.capacity.budgetUsed} cap={data.capacity.budgetCap} color="#222" />
          </View>
          <View style={st.bar}>
            {TIERS.map((t) => {
              const used = capOf(t)?.used ?? 0;
              const width = data.capacity.budgetCap > 0 ? (used / data.capacity.budgetCap) * 100 : 0;
              return <View key={t} style={{ width: `${Math.min(width, 100)}%`, backgroundColor: TIER_COLOR[t] }} />;
            })}
          </View>
          <Text style={s.sub}>
            {data.capacity.budgetUsed >= data.capacity.budgetCap
              ? 'Full. Accepting anyone new needs a seat freed first, whatever the tier.'
              : `${data.capacity.budgetCap - data.capacity.budgetUsed} seat${
                  data.capacity.budgetCap - data.capacity.budgetUsed === 1 ? '' : 's'
                } left across all tiers.`}
          </Text>
        </View>
      )}

      {data &&
        TIERS.map((t) => {
          const cap = capOf(t);
          const rows = byTier(t);
          return (
            <Section
              key={t}
              title={TIER_LABEL[t]}
              right={cap ? <Counter used={cap.used} cap={cap.cap} color={TIER_COLOR[t]} /> : null}
            >
              {rows.length === 0 && <Empty text="No one here yet." />}
              {rows.map((c) => (
                <View key={c.id} style={s.row}>
                  <View style={s.rowTop}>
                    <Text style={s.name}>{c.user.name}</Text>
                    <Button
                      label={confirmRemove === c.id ? 'Remove, sure?' : 'Remove'}
                      tone={confirmRemove === c.id ? 'danger' : 'default'}
                      disabled={busy === c.id}
                      onPress={() => remove(c)}
                    />
                  </View>
                  <TierPicker current={c.tier} disabled={busy === c.id} onPick={(tier) => void move(c, tier)} />
                  <Refusal text={refusals[c.id]} />
                </View>
              ))}
            </Section>
          );
        })}
    </ScrollView>
  );
}

const st = StyleSheet.create({
  body: { padding: 20, gap: 20, paddingBottom: 40 },
  budget: { gap: 8, padding: 14, borderRadius: 12, backgroundColor: '#f4f4f2' },
  budgetTitle: { fontSize: 15, fontWeight: '600', color: '#222' },
  bar: { flexDirection: 'row', height: 8, borderRadius: 4, overflow: 'hidden', backgroundColor: '#e0e0dc' },
});
