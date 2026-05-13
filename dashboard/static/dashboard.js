function api(url, opts) {
    opts = opts || {};
    opts.credentials = 'same-origin';
    if (!opts.headers) opts.headers = {};
    opts.headers['Accept'] = 'application/json';
    return fetch(url, opts).then(function(res) {
        if (res.status === 401) { window.location = '/dashboard/login'; throw new Error('unauthorized'); }
        if (!res.ok) { return res.text().then(function(t) { throw new Error(t) }) }
        return res.json();
    });
}
function esc(s) { return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }
function fmtNum(n) { if (!n) return '0'; if (n >= 1000000) return (n/1000000).toFixed(1)+'M'; if (n >= 1000) return (n/1000).toFixed(1)+'K'; return n.toString(); }
function fmtBytes(n) { if (!n||n===0) return '0 B'; var u=['B','KB','MB','GB','TB']; var i=Math.floor(Math.log(n)/Math.log(1024)); return (n/Math.pow(1024,i)).toFixed(1)+' '+u[i]; }
function fmtUptime(s) { if (!s||s<=0) return '0s'; var d=Math.floor(s/86400),h=Math.floor((s%86400)/3600),m=Math.floor((s%3600)/60),p=[]; if(d)p.push(d+'d');if(h)p.push(h+'h');if(m)p.push(m+'m');return p.join(' ')||(s+'s'); }
function fmtTime(ts) { if (!ts) return '-'; var d=new Date(ts); if(isNaN(d.getTime())) d=new Date(ts*1000); if(isNaN(d.getTime())) return '-'; return d.toLocaleString(); }
function showPwdModal() { document.getElementById('pwd-modal').style.display='flex'; document.getElementById('pwd-old').value=''; document.getElementById('pwd-new').value=''; hidePwdError(); }
function closePwdModal() { document.getElementById('pwd-modal').style.display='none'; }
function hidePwdError() { var e=document.getElementById('pwd-error'); e.style.display='none'; e.textContent=''; }
function changePassword() {
    var oldPwd = document.getElementById('pwd-old').value;
    var newPwd = document.getElementById('pwd-new').value;
    if (!oldPwd || !newPwd) { var e=document.getElementById('pwd-error'); e.style.display='block'; e.textContent='Both fields are required'; return; }
    if (newPwd.length < 4) { var e=document.getElementById('pwd-error'); e.style.display='block'; e.textContent='Password must be at least 4 characters'; return; }
    api('/dashboard/profile/password', {
        method: 'POST',
        body: JSON.stringify({old_password: oldPwd, new_password: newPwd})
    }).then(function() {
        closePwdModal();
        showToast('Password changed', 'success');
    }).catch(function(err) {
        var e=document.getElementById('pwd-error'); e.style.display='block'; e.textContent=err.message;
    });
}
function showToast(msg, type) {
    var t=document.createElement('div'); t.className='toast toast-'+(type||'success'); t.textContent=msg; document.body.appendChild(t);
    setTimeout(function(){ t.style.opacity='0'; setTimeout(function(){ t.remove() },300) },2000);
}