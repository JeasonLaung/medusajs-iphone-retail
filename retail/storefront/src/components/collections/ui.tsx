import {forwardRef,type HTMLAttributes,type InputHTMLAttributes,type LabelHTMLAttributes} from 'react'
import clsx from 'clsx'
export {clsx as clx}
export {Text} from '../search/ui'
export const Container=forwardRef<HTMLDivElement,HTMLAttributes<HTMLDivElement>>(({className,children,...props},ref)=><div ref={ref} className={clsx('bg-white rounded-lg p-4',className)} {...props}>{children}</div>)
Container.displayName='Container'
export const Label=forwardRef<HTMLLabelElement,LabelHTMLAttributes<HTMLLabelElement>>(({className,children,...props},ref)=><label ref={ref} className={clsx('text-sm font-medium',className)} {...props}>{children}</label>)
Label.displayName='Label'
const Root=forwardRef<HTMLDivElement,HTMLAttributes<HTMLDivElement>>(({className,children,...props},ref)=><div ref={ref} className={clsx('flex flex-col gap-2',className)} {...props}>{children}</div>)
Root.displayName='RadioGroup'
const Item=forwardRef<HTMLInputElement,InputHTMLAttributes<HTMLInputElement>>(({className,...props},ref)=><div className="flex items-center gap-2"><input ref={ref} type="radio" name="collection-sort" className={clsx('h-4 w-4 border-gray-300 text-gray-900 focus:ring-gray-900',className)} {...props}/></div>)
Item.displayName='RadioGroupItem'
export const RadioGroup=Object.assign(Root,{Item})
