import styles from './styles.module.css'
export default function ErrorMessage({error,'data-testid':testID}:{error?:string|null;'data-testid'?:string}){return error?<p role="alert" className={styles.error} data-testid={testID}>{error}</p>:null}
