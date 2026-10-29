import type {ReactNode} from 'react'
import {useFormStatus} from 'react-dom'
export function SubmitButton({children,className,'data-testid':testID}:{children:ReactNode;className?:string;'data-testid'?:string}){const {pending}=useFormStatus();return <button type="submit" className={className} disabled={pending} aria-busy={pending} data-testid={testID}>{children}</button>}
