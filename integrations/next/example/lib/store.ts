"use client";
// A deliberately tiny in-memory store. State lives for the life of the tab
// and resets on every full page load, so every `agent-browser open` — and
// every replayed plan — starts from the same three tasks. Client-side
// navigation (next/link) keeps it, which is what the detail page relies on.
import { useSyncExternalStore } from "react";

export type Task = { id: number; title: string; done: boolean };

const SEED: Task[] = [
  { id: 1, title: "Write the sightmap", done: false },
  { id: 2, title: "Compile the tool layer", done: false },
  { id: 3, title: "Ship to Vercel", done: true },
];

let tasks: Task[] = SEED;
let nextId = 4;
const listeners = new Set<() => void>();

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}
function commit(next: Task[]) {
  tasks = next;
  for (const fn of listeners) fn();
}

export function useTasks(): Task[] {
  return useSyncExternalStore(
    subscribe,
    () => tasks,
    () => SEED,
  );
}

export const store = {
  add(title: string) {
    const t = title.trim();
    if (!t) return;
    commit([...tasks, { id: nextId++, title: t, done: false }]);
  },
  toggle(id: number) {
    commit(tasks.map((t) => (t.id === id ? { ...t, done: !t.done } : t)));
  },
  complete(id: number) {
    commit(tasks.map((t) => (t.id === id ? { ...t, done: true } : t)));
  },
  remove(id: number) {
    commit(tasks.filter((t) => t.id !== id));
  },
};
