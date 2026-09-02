import { useState } from 'react';
import { Pressable, SafeAreaView, ScrollView, StyleSheet, Text, View } from 'react-native';
import { StatusBar } from 'expo-status-bar';

import { useLoad } from './src/hooks';
import { api } from './src/queries';
import { PeopleScreen } from './src/screens/PeopleScreen';
import { RequestsScreen } from './src/screens/RequestsScreen';
import { Button, Refusal } from './src/ui';

type Tab = 'people' | 'requests';

/**
 * A user switcher on top (there is no auth, by design), two tabs under it.
 * Each tab loads fresh for whoever you are acting as.
 */
export default function App() {
  const users = useLoad(() => api.users(), []);
  const [chosen, setChosen] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('people');

  const me = chosen ?? users.data?.users[0]?.id ?? null;

  return (
    <SafeAreaView style={st.screen}>
      <StatusBar style="auto" />

      <View style={st.header}>
        <Text style={st.label}>Acting as</Text>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={st.chips}>
          {(users.data?.users ?? []).map((u) => {
            const active = u.id === me;
            return (
              <Pressable key={u.id} onPress={() => setChosen(u.id)} style={[st.chip, active && st.chipActive]}>
                <Text style={[st.chipText, active && st.chipTextActive]}>{u.name}</Text>
              </Pressable>
            );
          })}
        </ScrollView>
      </View>

      {users.error && (
        <View style={st.down}>
          <Refusal text={users.error} />
          <Text style={st.hint}>Is the API up? `make up` then `make api`.</Text>
          <Button label="Retry" onPress={() => void users.reload()} />
        </View>
      )}

      {me && (
        <>
          <View style={st.tabs}>
            {(['people', 'requests'] as Tab[]).map((t) => (
              <Pressable key={t} onPress={() => setTab(t)} style={[st.tab, tab === t && st.tabActive]}>
                <Text style={[st.tabText, tab === t && st.tabTextActive]}>{t === 'people' ? 'People' : 'Requests'}</Text>
              </Pressable>
            ))}
          </View>
          {tab === 'people' ? <PeopleScreen key={me} me={me} /> : <RequestsScreen key={me} me={me} />}
        </>
      )}
    </SafeAreaView>
  );
}

const st = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#fff' },
  header: { paddingTop: 12, paddingHorizontal: 20, gap: 8 },
  label: { fontSize: 13, textTransform: 'uppercase', letterSpacing: 1, color: '#888' },
  chips: { gap: 8, paddingBottom: 4 },
  chip: { paddingVertical: 6, paddingHorizontal: 12, borderRadius: 999, borderWidth: 1, borderColor: '#e3e3e3' },
  chipActive: { borderColor: '#16605c', backgroundColor: '#e1efed' },
  chipText: { fontSize: 14, color: '#333' },
  chipTextActive: { color: '#16605c', fontWeight: '600' },
  tabs: { flexDirection: 'row', marginTop: 12, borderBottomWidth: 1, borderBottomColor: '#eee' },
  tab: { flex: 1, paddingVertical: 12, alignItems: 'center', borderBottomWidth: 2, borderBottomColor: 'transparent' },
  tabActive: { borderBottomColor: '#16605c' },
  tabText: { fontSize: 15, color: '#777' },
  tabTextActive: { color: '#16605c', fontWeight: '600' },
  down: { padding: 20, gap: 10 },
  hint: { color: '#9a3b2e', fontSize: 12, opacity: 0.8 },
});
