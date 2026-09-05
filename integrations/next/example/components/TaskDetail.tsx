"use client";
import Link from "next/link";
import { store, useTasks } from "@/lib/store";

export function TaskDetail({ id }: { id: number }) {
  const task = useTasks().find((t) => t.id === id);
  if (!task) {
    return (
      <article data-component="TaskMissing">
        <p>No task #{id}.</p>
        <Link data-component="BackToBoardLink" href="/">
          Back to the board
        </Link>
      </article>
    );
  }
  const status = task.done ? "Done" : "Active";
  return (
    <article data-component="TaskDetail" data-status={status}>
      <Link data-component="BackToBoardLink" href="/">
        ← Back to the board
      </Link>
      <h1 data-component="TaskTitle">{task.title}</h1>
      <p>
        <span data-component="StatusBadge">{status}</span>
        <button
          type="button"
          data-component="MarkDoneButton"
          disabled={task.done}
          onClick={() => store.complete(task.id)}
        >
          Mark done
        </button>
      </p>
    </article>
  );
}
