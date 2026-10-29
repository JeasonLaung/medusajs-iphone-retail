import type {ComponentProps} from 'react'
import Link from '../products/localized-link'
import {useAccountContext} from './context'
export default function AccountLink(props:Omit<ComponentProps<typeof Link>,'segment'>){return <Link {...props} segment={useAccountContext().segment}/>}
