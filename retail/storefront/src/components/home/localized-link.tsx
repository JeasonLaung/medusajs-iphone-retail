import {createContext,useContext,type ReactNode,type ComponentProps} from 'react'
import Link from '../products/localized-link'
const Context=createContext('')
export function HomeRoute({segment,children}:{segment:string;children:ReactNode}){return <Context.Provider value={segment}>{children}</Context.Provider>}
export default function HomeLink(props:Omit<ComponentProps<typeof Link>,'segment'>){return <Link {...props} segment={useContext(Context)}/>}
