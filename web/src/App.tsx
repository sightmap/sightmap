import { BrowserRouter, Routes, Route } from 'react-router'
import Home from '@/pages/Home'
import PasswordGate from '@/components/PasswordGate'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<PasswordGate><Home /></PasswordGate>} />
      </Routes>
    </BrowserRouter>
  )
}
