"use client";
import Link from "next/link";
import { useState } from "react";
import { store, useTasks } from "@/lib/store";

const FILTERS = ["All", "Active", "Done"] as const;
type Filter = (typeof FILTERS)[number];

export function TaskBoard() {
  const tasks = useTasks();
  const [filter, setFilter] = useState<Filter>("All");
  const [draft, setDraft] = useState("");
  const visible = tasks.filter((t) =>
    filter === "All" ? true : filter === "Done" ? t.done : !t.done,
  );

  return (
    <section data-component="TaskBoard" aria-label="Task board">
      <h1>Tasks</h1>
      <form
        data-component="NewTaskForm"
        onSubmit={(e) => {
          e.preventDefault();
          store.add(draft);
          setDraft("");
        }}
      >
        <input
          data-component="NewTaskInput"
          aria-label="New task title"
          placeholder="What needs doing?"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <button data-component="AddTaskButton" type="submit">
          Add task
        </button>
      </form>

      <div data-component="FilterBar" role="group" aria-label="Filter tasks">
        {FILTERS.map((f) => (
          <button
            key={f}
            type="button"
            data-component="FilterButton"
            data-filter={f}
            aria-pressed={f === filter}
            onClick={() => setFilter(f)}
          >
            {f}
          </button>
        ))}
      </div>

      <ul data-component="TaskList">
        {visible.map((t) => (
          <li
            key={t.id}
            data-component="TaskRow"
            data-id={t.id}
            data-done={String(t.done)}
          >
            <button
              type="button"
              data-component="TaskToggle"
              aria-label={`${t.done ? "Reopen" : "Complete"} ${t.title}`}
              onClick={() => store.toggle(t.id)}
            >
              {t.done ? "Undo" : "Done"}
            </button>
            <Link data-component="TaskLink" href={`/tasks/${t.id}`}>
              {t.title}
            </Link>
            <button
              type="button"
              data-component="DeleteTaskButton"
              aria-label={`Delete ${t.title}`}
              onClick={() => store.remove(t.id)}
            >
              Delete
            </button>
          </li>
        ))}
      </ul>
      {visible.length === 0 && <p data-component="EmptyState">Nothing here.</p>}
    </section>
  );
}
