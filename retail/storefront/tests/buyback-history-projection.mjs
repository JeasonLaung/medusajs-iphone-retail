// Private DTO arrives only on stdin; never log customer data or parse errors.
import {buybackRequestProjection} from '../src/lib/buyback-requests.ts'
try {
 let raw=''
 for await(const chunk of process.stdin) {
  raw+=chunk
  if(raw.length>2*1024*1024)throw Error('limit')
 }
 const request=buybackRequestProjection(JSON.parse(raw))
 if(request.status!=='photo_review'||request.photos.length!==6)throw Error('coverage')
 process.stdout.write(JSON.stringify({actual_admin_projection:true,photos:request.photos.length,items:request.items.length}))
}catch{
 process.stderr.write('Actual historical DTO projection failed; private input suppressed\n')
 process.exitCode=1
}
