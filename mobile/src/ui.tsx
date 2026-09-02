import type { ReactNode } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { TIER_COLOR, TIER_LABEL, TIER_TINT, TIERS, type Tier } from './types';

export function Section({ title, right, children }: { title: string; right?: ReactNode; children: ReactNode }) {
  return (
    <View style={s.section}>
      <View style={s.sectionHead}>
        <Text style={s.sectionTitle}>{title}</Text>
        {right}
      </View>
      {children}
    </View>
  );
}

/** used / cap. Red when used is at or over cap: the server may legally report more than cap. */
export function Counter({ used, cap, color }: { used: number; cap: number; color?: string }) {
  const full = used >= cap;
  return (
    <Text style={[s.counter, { color: color ?? '#555' }, full && s.counterFull]}>
      {used} / {cap}
      {used > cap ? '  over' : ''}
    </Text>
  );
}

export function Refusal({ text }: { text?: string }) {
  if (!text) return null;
  return (
    <View style={s.refusal}>
      <Text style={s.refusalText}>{text}</Text>
    </View>
  );
}

export function Empty({ text }: { text: string }) {
  return <Text style={s.empty}>{text}</Text>;
}

export function Button({
  label,
  onPress,
  tone = 'default',
  disabled,
}: {
  label: string;
  onPress: () => void;
  tone?: 'default' | 'primary' | 'danger';
  disabled?: boolean;
}) {
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [s.btn, s[`btn_${tone}`], (pressed || disabled) && s.btnDim]}
    >
      <Text style={[s.btnText, tone === 'default' && s.btnTextDark]}>{label}</Text>
    </Pressable>
  );
}

/**
 * Three pills, one per tier. `current` is highlighted; pressing another calls
 * onPick. Used both to choose a tier for a new request and to re-file a contact.
 */
export function TierPicker({
  current,
  onPick,
  disabled,
}: {
  current?: Tier;
  onPick: (t: Tier) => void;
  disabled?: boolean;
}) {
  return (
    <View style={s.pills}>
      {TIERS.map((t) => {
        const active = t === current;
        return (
          <Pressable
            key={t}
            disabled={disabled || active}
            onPress={() => onPick(t)}
            style={[s.pill, { borderColor: TIER_COLOR[t] }, active && { backgroundColor: TIER_COLOR[t] }]}
          >
            <Text style={[s.pillText, { color: active ? '#fff' : TIER_COLOR[t] }]}>{TIER_LABEL[t]}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

export function TierTag({ tier }: { tier: Tier }) {
  return (
    <View style={[s.tag, { backgroundColor: TIER_TINT[tier] }]}>
      <Text style={[s.tagText, { color: TIER_COLOR[tier] }]}>{TIER_LABEL[tier]}</Text>
    </View>
  );
}

export const s = StyleSheet.create({
  section: { gap: 8 },
  sectionHead: { flexDirection: 'row', alignItems: 'baseline', justifyContent: 'space-between' },
  sectionTitle: { fontSize: 13, textTransform: 'uppercase', letterSpacing: 1, color: '#888' },
  counter: { fontSize: 13, fontVariant: ['tabular-nums'] },
  counterFull: { fontWeight: '700', color: '#b23b2e' },
  refusal: { backgroundColor: '#fbeae7', borderRadius: 8, padding: 10, marginTop: 6 },
  refusalText: { color: '#9a3b2e', fontSize: 13, lineHeight: 18 },
  empty: { color: '#999', fontSize: 14, paddingVertical: 6 },
  row: {
    padding: 12,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: '#e3e3e3',
    gap: 8,
  },
  rowTop: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 8 },
  name: { fontSize: 16, color: '#222', flexShrink: 1 },
  sub: { fontSize: 13, color: '#777' },
  actions: { flexDirection: 'row', gap: 8, alignItems: 'center' },
  btn: { paddingVertical: 7, paddingHorizontal: 12, borderRadius: 7 },
  btn_default: { backgroundColor: '#eee' },
  btn_primary: { backgroundColor: '#16605c' },
  btn_danger: { backgroundColor: '#9a3b2e' },
  btnDim: { opacity: 0.5 },
  btnText: { color: '#fff', fontSize: 13, fontWeight: '600' },
  btnTextDark: { color: '#333' },
  pills: { flexDirection: 'row', gap: 6 },
  pill: { borderWidth: 1, borderRadius: 999, paddingVertical: 4, paddingHorizontal: 10 },
  pillText: { fontSize: 12, fontWeight: '600' },
  tag: { borderRadius: 6, paddingVertical: 2, paddingHorizontal: 8 },
  tagText: { fontSize: 12, fontWeight: '600' },
});
