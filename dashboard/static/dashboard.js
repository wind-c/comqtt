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