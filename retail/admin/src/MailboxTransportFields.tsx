import {Checkbox,Form,Input,InputNumber,Select,Typography} from 'antd'
import type {FormInstance} from 'antd'
import type {Mailbox} from './api'

export default function MailboxTransportFields({form,account,zh}:{form:FormInstance;account?:Mailbox;zh:boolean}){
 const provider=Form.useWatch(['transport','provider'],form)??'resend'
 const imap=Form.useWatch('imap_enabled',form)
 const keep=zh?'留空保留已保存凭据':'Leave blank to keep the saved credential'
 const required={required:true,message:zh?'必填':'Required'}
 return <>
  <Form.Item name={['transport','provider']} label={zh?'发信渠道':'Sending provider'} rules={[required]}><Select options={[{value:'resend',label:'Resend'},{value:'smtp',label:'SMTP'},{value:'quguo',label:zh?'趣果 Relayme API':'Relayme API'}]}/></Form.Item>
  {provider==='smtp'&&<>
   <Form.Item name={['transport','smtp','host']} label={zh?'SMTP 服务器':'SMTP host'} rules={[required]}><Input maxLength={255}/></Form.Item>
   <Form.Item name={['transport','smtp','port']} label={zh?'SMTP 端口':'SMTP port'} rules={[required]}><InputNumber min={1} max={65535}/></Form.Item>
   <Form.Item name={['transport','smtp','username']} label={zh?'SMTP 用户名':'SMTP username'}><Input maxLength={255}/></Form.Item>
   <Form.Item name={['transport','smtp','password']} label={zh?'SMTP 密码 / 授权码':'SMTP password'} extra={account?.transport?.has_smtp_password?keep:undefined}><Input.Password autoComplete="new-password" maxLength={4096}/></Form.Item>
   <Form.Item name={['transport','smtp','encryption']} label={zh?'SMTP 加密':'SMTP encryption'} rules={[required]}><Select options={[{value:'tls',label:'SSL/TLS'},{value:'starttls',label:'STARTTLS'},{value:'none',label:zh?'无':'None'}]}/></Form.Item>
   <Form.Item name={['transport','smtp','skip_tls_verify']} valuePropName="checked"><Checkbox>{zh?'跳过 SMTP 证书验证':'Skip SMTP certificate verification'}</Checkbox></Form.Item>
  </>}
  {provider==='quguo'&&<>
   <Form.Item name={['transport','relayme','api_url']} label={zh?'趣果 API 根地址':'Relayme API root URL'} rules={[required,{type:'url',message:zh?'请输入完整地址':'Enter a complete URL'}]}><Input maxLength={500}/></Form.Item>
   <Form.Item name={['transport','relayme','key_id']} label="Key ID" rules={[required]}><Input maxLength={128}/></Form.Item>
   <Form.Item name={['transport','relayme','secret']} label="Secret" extra={account?.transport?.has_relayme_secret?keep:undefined} rules={[{required:!account?.transport?.has_relayme_secret,message:required.message}]}><Input.Password autoComplete="new-password" maxLength={4096}/></Form.Item>
  </>}
  <Form.Item name="imap_enabled" valuePropName="checked"><Checkbox>{zh?'配置 IMAP 收件':'Configure IMAP receiving'}</Checkbox></Form.Item>
  {imap&&<>
   <Typography.Text strong>{zh?'IMAP 收件配置':'IMAP receiving configuration'}</Typography.Text>
   <Form.Item name={['transport','imap','host']} label={zh?'IMAP 服务器':'IMAP host'} rules={[required]}><Input maxLength={255}/></Form.Item>
   <Form.Item name={['transport','imap','port']} label={zh?'IMAP 端口':'IMAP port'} rules={[required]}><InputNumber min={1} max={65535}/></Form.Item>
   <Form.Item name={['transport','imap','username']} label={zh?'IMAP 用户名':'IMAP username'} rules={[required]}><Input maxLength={255}/></Form.Item>
   <Form.Item name={['transport','imap','password']} label={zh?'IMAP 密码 / 授权码':'IMAP password'} extra={account?.transport?.has_imap_password?keep:undefined} rules={[{required:!account?.transport?.has_imap_password,message:required.message}]}><Input.Password autoComplete="new-password" maxLength={4096}/></Form.Item>
   <Form.Item name={['transport','imap','encryption']} label={zh?'IMAP 加密':'IMAP encryption'} rules={[required]}><Select options={[{value:'tls',label:'SSL/TLS'},{value:'starttls',label:'STARTTLS'}]}/></Form.Item>
   <Form.Item name={['transport','imap','cert_sha256']} label={zh?'证书 SHA256 指纹（可选）':'Certificate SHA256 pin (optional)'} rules={[{pattern:/^$|^[0-9a-fA-F]{64}$/,message:zh?'请输入 64 位十六进制指纹':'Enter a 64 character hexadecimal fingerprint'}]}><Input maxLength={64}/></Form.Item>
  </>}
 </>
}
