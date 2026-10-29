"use client"

import { useState } from "react"

import Register from "./register"
import Login from "./login"

export enum LOGIN_VIEW {
  SIGN_IN = "sign-in",
  REGISTER = "register",
}

const LoginTemplate = ({ onSuccess }: { onSuccess?: () => void }) => {
  const [currentView, setCurrentView] = useState("sign-in")

  return (
    <div className="w-full flex justify-center px-8 py-8">
      {currentView === "sign-in" ? (
        <Login setCurrentView={setCurrentView} onSuccess={onSuccess} />
      ) : (
        <Register setCurrentView={setCurrentView} onSuccess={onSuccess} />
      )}
    </div>
  )
}

export default LoginTemplate
