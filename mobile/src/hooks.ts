import { useCallback, useEffect, useRef, useState } from 'react';

import { messageOf } from './api';

/**
 * Load something for the current user and reload it on demand. Every mutation
 * on a screen ends with reload(), so used / cap on screen is always what the
 * server just counted, never a number the client kept for itself.
 */
export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const seq = useRef(0);

  const reload = useCallback(async () => {
    const mine = ++seq.current;
    setLoading(true);
    try {
      const next = await load();
      if (mine === seq.current) {
        setData(next);
        setError(null);
      }
    } catch (e) {
      if (mine === seq.current) setError(messageOf(e));
    } finally {
      if (mine === seq.current) setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { data, error, loading, reload };
}

/**
 * Run mutations from rows on a list, remembering which row is busy and the
 * sentence the server refused it with, so the reason lands under the button
 * that was pressed and nowhere else.
 */
export function useRowActions(afterSuccess: () => Promise<void> | void) {
  const [busy, setBusy] = useState<string | null>(null);
  const [refusals, setRefusals] = useState<Record<string, string>>({});

  const run = useCallback(
    async (rowId: string, action: () => Promise<unknown>) => {
      setBusy(rowId);
      setRefusals((r) => {
        const { [rowId]: _gone, ...rest } = r;
        return rest;
      });
      try {
        await action();
        await afterSuccess();
      } catch (e) {
        setRefusals((r) => ({ ...r, [rowId]: messageOf(e) }));
      } finally {
        setBusy(null);
      }
    },
    [afterSuccess],
  );

  return { busy, refusals, run };
}
