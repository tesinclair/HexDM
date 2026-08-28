Container::Root{
    Container::VBox(gap=2){
        Widget::Text<title>(font_size=5, text="HexDM", focus=false)
        Container::Carousel<users>(n=3, gap=2)
        Container::Pager<auth_pager>{
            Container::VBox(gap=3){
                Widget::Box<fprint>{
                    Container::Stack{
                        Container::Path<fp_ring>(curve=Ellipse{extent={4,4}}, offset=0.0, span=0.25, opacity=0.0){}
                        Widget::Animation<fprint_anim>(focus=false)
                    }
                }
                Container::HBox<remaining_tries>(gap=0.5){
                }
            }
            Widget::Box<password>{
                Container::Stack{
                    Container::Path<pw_ring>(curve=Rect{extent={0,10}}, offset=0.0, span=0.10, opacity=0.0){}
                    Container::HBox<password_text>(focus=false, gap=1)
                }
            }
        }
    }
    Container::Box(align={center,bottom}, offset={0,-2}, focus=false){
        Container::VBox(gap=1){
            Widget::Text<caps_warn>(text="CAPS LOCK", font_size=2, opacity=0.0, focus=false)
            Container::Carousel<sessions>(n=5)
            Container::HBox(gap=2){
                Widget::Box<reboot_btn>{ Widget::Text(text="Reboot", focus=false) }
                Widget::Box<shutdown_btn>{ Widget::Text(text="Shutdown", focus=false) }
            }
        }
    }
}

Auth::Fprint(fprint)
Auth::Password(password)

State::CapsLock(Root => caps_warn){ opacity=1.0 }
State::FingerScanning(fprint => fp_ring){ opacity=1.0 }
State::AuthSuccess(password => pw_ring){ opacity=1.0 }
  : (password){ border="#00FF00" }
State::AuthFail(fprint => fprint_anim){ animation="failure" }
  : (password){ border="#FF0000" }
State::Focus(reboot_btn){ border="#AAAAAA" }
  : (shutdown_btn){ border="#AAAAAA" }

Trigger::FprintDisabled(fprint => auth_pager){ active_page = 1 }

Script::Startup(=> users as u){
    Daemon::LinkCarousel(u, Daemon::User)
} : (=> sessions as s){
    Daemon::LinkCarousel(s, Daemon::Session)
} : (=> password_text as pt){
    Daemon::LinkRepeat(pt, Daemon::PasswordLength, Widget::Circle(radius=0.35, color="#DDDDDD"))
} : (=> remaining_tries as rt){
    Daemon::LinkRepeat(rt, Daemon::FprintRemainingTries, Container::Stack{
        Widget::Line(transform_rotate=45, color="#FF0000")
        Widget::Line(transform_rotate=-45, color="#FF0000")
    })
} : (=> reboot_btn as rb){
    Daemon::LinkAccept(rb, Daemon::Reboot)
} : (=> shutdown_btn as sb){
    Daemon::LinkAccept(sb, Daemon::Shutdown)
} : (=> pw_ring as pwr){
    for i in 0..<24{
        Daemon::AddChild(pwr, Widget::Pixel(color="#00FF00", opacity=1.0 - i / 24.0))
    }
} : (=> fp_ring as fpr){
    for i in 0..<16{
        Daemon::AddChild(fpr, Widget::Pixel(color="#66CCFF", opacity=1.0 - i / 16.0))
    }
}

Script::Frame(1 => title){
    title.color = Util::HSVToHex(Math::Mod(Daemon::GetTime() * 20.0, 360.0), 0.45, 1.0)
} : (=> fp_ring as fr){
    if Daemon::InState(FingerScanning){
        fr.offset = Math::Mod(Daemon::GetTime() * 0.8, 1.0)
    }
} : (=> password as pw){
    pw.offset_x = Math::Sin(Daemon::TimeSince(AuthFail) * 42.0)
                * Math::Max(0.0, 0.35 - Daemon::TimeSince(AuthFail)) * 2.0
} : (=> pw_ring as ring){
    if Daemon::InState(AuthSuccess){
        ring.offset = Math::Mod(Daemon::TimeSince(AuthSuccess) * 1.6, 1.0)
        ring.span   = Math::Min(0.10 + Daemon::TimeSince(AuthSuccess) * 0.9, 1.0)
    }
} : (30 => Root as r){
    r.opacity = Math::Max(0.35, 1.0 - Math::Max(0.0, Daemon::IdleTime() - 20.0) * 0.05)
}
