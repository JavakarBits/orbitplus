package cache

import (
	"fmt"

	"github.com/orbitcacheservices/errors"
	"github.com/orbitcacheservices/models"
	"github.com/orbitcacheservices/search"
	"github.com/orbitcacheservices/utils/date_utils"
)

func BitsTripCreateOrUpdate(t models.BitsTripBleve) errors.RestErrors {
	key := fmt.Sprintf("%s_%s_%s", t.FromStationCode, t.ToStationCode, date_utils.GetFileDateLayoutFormat(t.TripDate))
	t.TripKey = key
	t.ModifiedDate = date_utils.GetNowDBFormat()
	search.BitsSearchResultIndex.Index(string(key), t.SearchResults)
	return nil
}

func BitsBusMapCreateOrUpdate(t models.BitsBusMapBleve) errors.RestErrors {
	key := fmt.Sprintf("%s_%s ", t.TripCode, t.OperatorCode)
	t.Key = key
	t.ModifiedDate = date_utils.GetNowDBFormat()
	search.BitsBusMapIndex.Index(string(key), t)
	return nil
}

func OperatorRoutesCreateOrUpdate(t models.OperatorRouteBleve) errors.RestErrors {
	key := fmt.Sprintf("%s_%s", t.OperatorCode, "Routes")
	t.ModifiedDate = date_utils.GetNowDBFormat()
	search.OperatorRoutesIndex.Index(string(key), t)
	return nil
}
