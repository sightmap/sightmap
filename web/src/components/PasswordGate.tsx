import { useState, type ReactNode, type FormEvent } from 'react'

const STORAGE_KEY = 'sightmap_access'
const PASSWORD = 'argus'

export default function PasswordGate({ children }: { children: ReactNode }) {
  const [authed, setAuthed] = useState(() => localStorage.getItem(STORAGE_KEY) === 'granted')
  const [value, setValue] = useState('')
  const [error, setError] = useState(false)

  if (authed) return <>{children}</>

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (value === PASSWORD) {
      localStorage.setItem(STORAGE_KEY, 'granted')
      setAuthed(true)
    } else {
      setError(true)
      setValue('')
    }
  }

  return (
    <div data-component="PasswordGate" style={{
      display: 'flex',
      minHeight: '100vh',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'var(--bg)',
    }}>
      <form
        onSubmit={handleSubmit}
        style={{
          width: '100%',
          maxWidth: '380px',
          margin: '0 1rem',
          padding: '2rem',
          background: 'var(--bg-raised)',
          border: '1px solid var(--border)',
          borderRadius: '12px',
        }}
      >
        <h2 style={{
          fontFamily: 'var(--sans)',
          fontSize: '1.25rem',
          fontWeight: 600,
          color: 'var(--text-bright)',
          letterSpacing: '-0.02em',
          marginBottom: '0.5rem',
        }}>
          Password protected
        </h2>
        <p style={{
          fontFamily: 'var(--sans)',
          fontSize: '0.9rem',
          color: 'var(--text-dim)',
          marginBottom: '1.5rem',
        }}>
          Enter the password to continue.
        </p>
        <input
          type="password"
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
            setError(false)
          }}
          placeholder="Password"
          autoFocus
          style={{
            width: '100%',
            padding: '0.75rem 1rem',
            marginBottom: '0.75rem',
            background: 'var(--bg-subtle)',
            border: '1px solid var(--border)',
            borderRadius: '8px',
            color: 'var(--text)',
            fontFamily: 'var(--mono)',
            fontSize: '0.88rem',
            outline: 'none',
          }}
          onFocus={(e) => e.currentTarget.style.borderColor = 'var(--accent)'}
          onBlur={(e) => e.currentTarget.style.borderColor = 'var(--border)'}
        />
        {error && (
          <p style={{
            fontSize: '0.85rem',
            color: 'var(--accent)',
            marginBottom: '0.75rem',
          }}>
            Incorrect password
          </p>
        )}
        <button
          type="submit"
          style={{
            width: '100%',
            padding: '0.75rem',
            background: 'var(--text-bright)',
            color: 'var(--bg)',
            fontFamily: 'var(--mono)',
            fontSize: '0.85rem',
            fontWeight: 500,
            border: 'none',
            borderRadius: '8px',
            cursor: 'pointer',
          }}
        >
          Submit
        </button>
      </form>
    </div>
  )
}
