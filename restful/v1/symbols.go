package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/adyzng/GoSymbols/restful"
	"github.com/adyzng/GoSymbols/symbol"
	"github.com/gorilla/mux"

	log "gopkg.in/clog.v1"
)

// RestBranchList response to restful API
//	[:]/api/branches  [GET]
//
//	@ return {
//		Total: 		int
//		Branchs: 	[]*symbol.Branch
//	}
//
func RestBranchList(w http.ResponseWriter, r *http.Request) {
	bs := restful.BranchList{}
	symbol.GetServer().WalkBuilders(func(bu symbol.Builder) error {
		if b, ok := bu.(*symbol.BrBuilder); ok {
			bs.Total++
			nb := b.Branch
			bs.Branchs = append(bs.Branchs, &nb)
		}
		return nil
	})
	resp := restful.RestResponse{
		Data: &bs,
	}
	resp.WriteJSON(w)
}

// RestBuildList response to restful API
//	[:]/api/branches/{name}  [GET]
//
//	@:name {branch name}
//
//	@return {
//		Total: 		int
//		Builds: 	[]*symbol.Build
//	}
//
func RestBuildList(w http.ResponseWriter, r *http.Request) {
	var vars = mux.Vars(r)
	resp := restful.RestResponse{
		ErrCodeMsg: restful.ErrInvalidParam,
	}

	if sname, ok := vars["name"]; ok {
		builder := symbol.GetServer().Get(sname)
		if builder != nil {
			blst := restful.BuildList{
				Branch: sname,
			}
			_, err := builder.ParseBuilds(func(build *symbol.Build) error {
				blst.Total++
				blst.Builds = append(blst.Builds, build)
				return nil
			})
			if err != nil {
				log.Error(2, "[Restful] Parse builds for %s failed: %v.", sname, err)
			}
			resp.Data = blst
			resp.ErrCodeMsg = restful.ErrSucceed
		} else {
			resp.ErrCodeMsg = restful.ErrUnknownBranch
		}
	}
	resp.WriteJSON(w)
}

// RestSymbolList response to restful API
//	[:]/api/branches/:name/:bid  [GET]
//
//	@:name {branch name}
//	@:bid  {build id}
//
//	@ return {
//		Total: 		int
//		Builds: 	[]*symbol.Build
//	}
//
func RestSymbolList(w http.ResponseWriter, r *http.Request) {
	var vars = mux.Vars(r)
	resp := restful.RestResponse{
		ErrCodeMsg: restful.ErrInvalidParam,
	}

	sname, bid := vars["name"], vars["bid"]
	if sname != "" && bid != "" {
		buider := symbol.GetServer().Get(sname)
		if buider != nil {
			symLst := restful.SymbolList{
				Branch: sname,
				Build:  bid,
			}
			_, err := buider.ParseSymbols(bid, func(sym *symbol.Symbol) error {
				symLst.Total++
				symLst.Symbols = append(symLst.Symbols, sym)
				return nil
			})
			if err != nil {
				log.Error(2, "[Restful] Parse symbols for %s:%s failed: %v.",
					sname, bid, err)
			}
			resp.Data = symLst
			resp.ErrCodeMsg = restful.ErrSucceed
		} else {
			resp.ErrCodeMsg.Message = "no such build"
		}
	}
	resp.WriteJSON(w)
}

// DownloadSymbol response download symbol file api
//	[:]/api/symbol/{branch}/{hash}/{name} [GET]
//
//	@:branch	{branch name}
//	@:hash		{file hash}
//	@:name		{file name}
//
//	@ return file
//
func DownloadSymbol(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bname := vars["branch"]
	fname := vars["name"]
	hash := vars["hash"]

	if bname == "" || hash == "" || fname == "" {
		log.Warn("[Restful] Download symbol invalid param: [%s, %s, %s]",
			bname, hash, fname)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	buider := symbol.GetServer().Get(bname)
	if buider == nil {
		log.Warn("[Restful] Download symbol branch not exist: [%s, %s, %s]",
			bname, hash, fname)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	fpath := buider.GetSymbolPath(hash, fname)
	fd, err := os.OpenFile(fpath, os.O_RDONLY, 666)
	if err != nil {
		log.Warn("[Restful] Open symbol file %s failed: %v.", fpath, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer fd.Close()

	// set response header
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fname))

	// send fil content
	var size int64
	if size, err = io.Copy(w, fd); err != nil {
		log.Error(2, "[Restful] Send file failed: %v.", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	//w.WriteHeader(http.StatusOK)
	log.Trace("[Restful] Send file complete. [%d: %s]", size, fpath)
}

// ValidateBranch response to check branch api
//	[:]/api/branch/check [POST]
//
//  @:BODY	{branch infomation}
//
//	@ return {
//		RestResponse
//	}
//
func ValidateBranch(w http.ResponseWriter, r *http.Request) {
	var branch symbol.Branch
	if err := json.NewDecoder(r.Body).Decode(&branch); err != nil {
		log.Error(2, "[Restful] Decode request body failed: %v.", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	resp := restful.RestResponse{}
	br := symbol.NewBranch2(&branch)
	if !br.CanUpdate() {
		resp.ErrCodeMsg = restful.ErrInvalidBranch
		resp.Message = "branch is not accessable from build server."
		resp.WriteJSON(w)
		return
	}

	if br.CanBrowse() {
		resp.ErrCodeMsg = restful.ErrExistOnLocal
		resp.WriteJSON(w)
		return
	}

	resp.WriteJSON(w)
}

// ModifyBranch response to modify branch api
//	[:]/api/branches/modify [POST]
//
//  @:BODY		{branch infomation}
//
//	@ return {
//		RestResponse
//	}
//
func ModifyBranch(w http.ResponseWriter, r *http.Request) {
	resp := restful.RestResponse{}
	ss := symbol.GetServer()

	var branch symbol.Branch
	if err := json.NewDecoder(r.Body).Decode(&branch); err != nil {
		log.Error(2, "[Restful] Decode request body failed: %v.", err)
		w.WriteHeader(http.StatusBadRequest)
	}

	if br := ss.Modify(&branch); br == nil {
		log.Warn("[Restful] Modify invalid branch %v.", branch)
		resp.ErrCodeMsg = restful.ErrInvalidBranch
		resp.WriteJSON(w)
		return
	}
	if err := ss.SaveBranchs(""); err != nil {
		log.Warn("[Restful] Save branch (%v) failed: %v.", branch, err)
	}
	resp.WriteJSON(w)
}

// DeleteBranch response to modify branch api
//	[:]/api/branches/{name} [DELETE]
//
//	@:name		{branch name}
//
//	@ return {
//		RestResponse
//	}
//
func DeleteBranch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bname := vars["name"]
	resp := restful.RestResponse{}

	branch := symbol.GetServer().Get(bname)
	if branch == nil {
		log.Warn("[Restful] Delete unknown branch %s.", bname)
		resp.ErrCodeMsg = restful.ErrUnknownBranch
		resp.WriteJSON(w)
	} else {
		resp.ErrCodeMsg = restful.ErrUnauthorized
		w.WriteHeader(http.StatusUnauthorized) // not allow for now
	}
}

// FetchTodayMsg get today symbols update information
//	[:]/api/messages [GET]
//
//	@ return {
//		RestResponse
//	}
//
func FetchTodayMsg(w http.ResponseWriter, r *http.Request) {
	resp := restful.RestResponse{}
	msgs := make([]*restful.Message, 0, 5)
	today := time.Now().Format("2006-01-02")

	symbol.GetServer().WalkBuilders(func(builder symbol.Builder) error {
		if b := builder.GetBranch(); b != nil {
			if strings.Index(b.UpdateDate, today) == 0 {
				msg := &restful.Message{
					Status: 1, // succeed
					Branch: b.StoreName,
					Build:  b.LatestBuild,
					Date:   b.UpdateDate,
				}
				msgs = append(msgs, msg)
			}
		}
		return nil
	})

	resp.Data = msgs
	resp.ErrCodeMsg = restful.ErrSucceed
	resp.WriteJSON(w)
}

// CreateBranch response to create branch api
//  [:]/api/branhes/create [POST]
//
//  @:BODY		{branch infomation}
//
//	@ return {
//		RestResponse
//	}
//
func CreateBranch(w http.ResponseWriter, r *http.Request) {
	_, token := loginRequired(r)
	if token == nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Warn("[Restful] Login required.")
		return
	}
	branch := symbol.Branch{}
	if err := json.NewDecoder(r.Body).Decode(&branch); err != nil {
		log.Error(2, "[Restful] Decode request body failed: %v.", err)
		w.WriteHeader(http.StatusBadRequest)
	}

	resp := restful.RestResponse{}
	br := symbol.GetServer().Add(&branch)
	if br == nil {
		log.Warn("[Restful] Create invalid branch %v.", branch)
		resp.ErrCodeMsg = restful.ErrInvalidBranch
		resp.WriteJSON(w)
		return
	}
	if !br.CanUpdate() {
		resp.Message = fmt.Sprintf("path not accessable (%s)", branch.BuildPath)
	} else {
		// trigger add new build
		go br.AddBuild("")
	}
	if !br.CanBrowse() {
		resp.Message = fmt.Sprintf("path not accessable (%s)", branch.StorePath)
	}
	log.Info("[Restful] User %s create branch %s.", token.UserName, br.Name())

	if err := symbol.GetServer().SaveBranchs(""); err != nil {
		log.Warn("[Restful] Save branch (%v) failed: %v.", branch, err)
	}
	resp.WriteJSON(w)
}
